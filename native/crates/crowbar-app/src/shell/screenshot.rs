//! Rendering the shell **to an image**, offscreen.
//!
//! # Why this exists
//!
//! Everything else in this port can measure the app but not *see* it.
//! `--inspect` reports geometry and colour for anchored boxes, and it is blind
//! to two whole classes of defect: artwork (an empty box and an icon are the
//! same record) and anything the surfaces do not anchor. That blindness cost
//! this session several confident, wrong claims that the sidebar looked right.
//!
//! Screen capture is not the answer — it needs an OS permission no process in
//! this tree can grant itself, and a workflow that ends in a human squinting
//! at a window does not scale and cannot be diffed.
//!
//! gpui can render a window to an image without a screen at all:
//! `HeadlessAppContext` drives a real Metal renderer against an offscreen
//! surface and hands back an `RgbaImage`. That is the renderer's own output,
//! so it sees exactly what a user would — icons included — and needs no
//! permission whatsoever.
//!
//! # What it renders
//!
//! The real [`super::Sidebar`] over a real [`SidebarStore`], with the same
//! fonts and the same asset source the shipping binary uses. Not a mock: a
//! screenshot of something other than the shipping view would be the same
//! mistake `crowbar_ui::anchor`'s `Unanchored` exists to avoid.

use std::path::Path;
use std::sync::Arc;

use crowbar_core::proto::api_v0_dto::{ProjectDTO, RepoDTO, WorkspaceDTO};
use crowbar_core::sidebar::cache::{Scope, Seed};
use crowbar_state::SidebarStore;
use crowbar_ui::gpui::{AppContext as _, HeadlessAppContext, Pixels, size};

use super::{Shell, Sidebar};

/// The desktop, stood in for.
///
/// The real window is transparent and its ground is the desktop seen through
/// vibrancy. A headless render has no desktop, and gpui's headless renderer
/// clears **opaque**, so there is no alpha to composite against afterwards
/// either — everything the app paints semi-transparently would land on black
/// and the whole image would read ~14 levels too dark.
///
/// L ≈ 0.16 in sRGB, derived rather than chosen: the Tauri app's own capture
/// reads `#272727` on bare sidebar ground, and `chrome_bg` is L 0.149 at
/// α 0.65, so whatever shows through the remaining 35% must be
/// `(0.153 - 0.149 * 0.65) / 0.35 ≈ 0.16`.
struct Backdrop {
    shell: crowbar_ui::gpui::Entity<Shell>,
}

impl crowbar_ui::gpui::Render for Backdrop {
    fn render(
        &mut self,
        _window: &mut crowbar_ui::gpui::Window,
        _cx: &mut crowbar_ui::gpui::Context<Self>,
    ) -> impl crowbar_ui::gpui::IntoElement {
        use crowbar_ui::gpui::{ParentElement as _, Styled as _, div};
        use crowbar_ui::theme::Color;

        // Derived from a token, not written as a literal — §4.3 rule 3, and
        // the check script is right to insist: a colour literal at a call
        // site is the one bypass a sealed newtype cannot catch.
        //
        // 16% white over black is L 0.16, which is what the arithmetic above
        // says must be behind `chrome_bg` for the composite to read `#272727`
        // as the Tauri app's own capture does.
        div()
            .size_full()
            .bg(Color::BLACK.mix(84.0, Color::WHITE).value())
            .child(self.shell.clone())
    }
}

/// Render the sidebar at `width` x `height` and write it to `out`.
///
/// # Panics
///
/// This is a test-only instrument; it panics rather than threading an error
/// nobody would handle.
pub fn render_sidebar_png(
    width: Pixels,
    height: Pixels,
    seed: impl FnOnce(&mut SidebarStore, &mut crowbar_ui::gpui::Context<SidebarStore>),
    out: &Path,
) {
    let platform = gpui_platform::current_platform(true);
    let mut cx = HeadlessAppContext::with_platform(
        platform.text_system(),
        Arc::new(crate::Assets),
        gpui_platform::current_headless_renderer,
    );

    // The same faces the shipping binary loads. Without them every string
    // shapes with a fallback and the image is a picture of the wrong font.
    cx.update(|cx| {
        crate::load_ui_font(cx);
        crate::load_ui_mono_font(cx);
        gpui_component::init(cx);
    });

    let store = cx.update(|cx| {
        let store = SidebarStore::build(cx, None, None, None);
        store.update(cx, |store, cx| seed(store, cx));
        store
    });

    let window = cx
        .open_window(size(width, height), |_window, cx| {
            // The whole **Shell**, not the sidebar alone. The shell is what
            // paints `chrome_bg` — the app's tint — and a picture of the
            // sidebar without it is a picture of the wrong ground, which is
            // how this instrument managed to look right while the app did
            // not.
            //
            // `Unanchored`, exactly as the shipping window uses: the image
            // has to be of the tree that ships, not of an instrumented one.
            let anchors = std::rc::Rc::new(crowbar_ui::Unanchored);
            let sidebar = Sidebar::build(&store, anchors.clone(), cx);
            let shell = cx.new(|_| Shell {
                sidebar,
                caption: "".into(),
                store: store.clone(),
                anchors,
            });
            cx.new(|_| Backdrop { shell })
        })
        .expect("a headless window opens");

    cx.run_until_parked();

    let image = cx
        .capture_screenshot(window.into())
        .expect("the headless renderer produced a frame");

    // Composited over a backdrop before it is written.
    //
    // The window is transparent and its frost is the *desktop* seen through
    // vibrancy — which a headless render has none of, so everything the app
    // paints semi-transparently lands on black and the whole picture reads
    // ~14 levels too dark. Measured against the Tauri app's own capture:
    // every region differed by exactly the ground's difference and no more,
    // and the one opaque element (the active project row) matched outright.
    //
    // So the difference was the missing desktop, not the painting. This
    // composites over a neutral stand-in for it, which is what makes the
    // image comparable to a capture of the real window — and what makes it a
    // fair picture of the app rather than a flattering or a damning one.
    image.save(out).expect("the png is written");
}

/// A project, two repos and a workspace each — the shape of the dev fixture,
/// so the image is of the app a reviewer is actually comparing against.
pub fn fixture_seed() -> impl FnOnce(&mut SidebarStore, &mut crowbar_ui::gpui::Context<SidebarStore>)
{
    |store, cx| {
        let generation = store.begin_reseed();
        store.apply_seed(
            &Scope::Projects,
            generation,
            Seed::Projects(vec![project("oracle-fixture")]),
            cx,
        );
        store.set_active_project(Some("oracle-fixture".to_string()), cx);

        let generation = store.begin_reseed();
        store.apply_seed(
            &Scope::Repos {
                project_id: "oracle-fixture".to_string(),
            },
            generation,
            Seed::Repos(vec![repo("demo"), repo("oracle-repo")]),
            cx,
        );

        // Each repo gets a default workspace as well as its tree row — the
        // shape the dev daemon actually serves. Without it `has_default_
        // workspace` is false and the repo header renders two trailing
        // actions where the reference renders three, which reads as a styling
        // difference and is not one.
        for repo_id in ["demo", "oracle-repo"] {
            let mut default = workspace("main", repo_id);
            default.id = format!("{repo_id}-default");
            default.is_default = Some(true);

            let mut row = workspace("main", repo_id);
            // `oracle-repo`'s branch is a placeholder in the fixture home, and
            // a placeholder draws the warning mark rather than the lock.
            if repo_id == "oracle-repo" {
                row.local_path = None;
                row.held_by_path = Some("/tmp/elsewhere".to_string());
                row.status = None;
            }

            let generation = store.begin_reseed();
            store.apply_seed(
                &Scope::Workspaces {
                    project_id: "oracle-fixture".to_string(),
                    repo_id: repo_id.to_string(),
                },
                generation,
                Seed::Workspaces(vec![default, row]),
                cx,
            );
        }
    }
}

fn project(id: &str) -> ProjectDTO {
    ProjectDTO {
        id: id.to_string(),
        name: id.to_string(),
        path: format!("/tmp/{id}"),
        status: None,
        last_activity: String::new(),
    }
}

fn repo(id: &str) -> RepoDTO {
    RepoDTO {
        id: id.to_string(),
        project_id: "oracle-fixture".to_string(),
        name: id.to_string(),
        path: format!("/tmp/oracle-fixture/{id}"),
        default_branch: "main".to_string(),
        avatar_label: id.chars().next().unwrap_or('R').to_uppercase().to_string(),
        avatar_color: "avatar-slate".to_string(),
        avatar_url: None,
        avatar_emoji: None,
        status: None,
    }
}

fn workspace(branch: &str, repo_id: &str) -> WorkspaceDTO {
    WorkspaceDTO {
        id: format!("{repo_id}-{branch}"),
        repo_id: repo_id.to_string(),
        project_id: "oracle-fixture".to_string(),
        kind: None,
        branch: branch.to_string(),
        parent_id: None,
        fork_point_sha: None,
        status: Some(crowbar_core::proto::domain::WorkspaceStatus::Locked),
        working: false,
        last_error: None,
        is_default: None,
        added: 0,
        deleted: 0,
        merge_strategy: crowbar_core::proto::domain_git::MergeStrategy::Other(String::new()),
        can_merge_locally: false,
        merge_conflicts: false,
        parent_branch: None,
        pr_url: None,
        pr_title: None,
        pr_target_branch: None,
        local_path: Some(format!("/tmp/{repo_id}")),
        held_by_path: None,
    }
}

#[cfg(test)]
mod tests {
    use super::{fixture_seed, render_sidebar_png};
    use crowbar_ui::gpui::px;

    /// Writes the sidebar to `target/sidebar.png`.
    ///
    /// Not an assertion — an **instrument**. Its output is the thing a
    /// reviewer actually looks at, and it needs no screen-recording permission
    /// because gpui renders it offscreen.
    ///
    /// # Why `#[ignore]`
    ///
    /// It drives Metal, and `AppKit`/Metal are main-thread-only. Rust runs
    /// tests on worker threads, so under a plain `cargo test` this aborts the
    /// whole binary with SIGABRT — taking every other test in it down, which
    /// reads as an unrelated catastrophe. gpui's own visual tests carry the
    /// same attribute for the same reason (`gpui_platform`'s test module says
    /// so in as many words).
    ///
    /// Run it deliberately:
    ///
    /// ```sh
    /// cargo test -p crowbar-app write_the_sidebar_png -- --ignored --test-threads=1
    /// ```
    #[test]
    #[ignore = "drives Metal, which is main-thread only; see the doc comment"]
    fn write_the_sidebar_png() {
        let out = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../target/sidebar.png");
        render_sidebar_png(px(294.0), px(600.0), fixture_seed(), &out);
        assert!(out.exists(), "no image was written to {}", out.display());
    }
}
