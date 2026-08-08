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
    /// Does gpui paint an **inset** box shadow at all?
    ///
    /// Isolation probe, written when `elevation::raised()`'s drop-shadow layer
    /// rendered and its inset layer did not. Everything upstream checked out —
    /// the colour arithmetic (`elevation`'s own tests), `paint_inset_shadows`
    /// being called unconditionally after the background, the Metal shader's
    /// zero-blur fast path, and the scene's `(order, kind)` sort — so the
    /// question left is whether a pixel comes out, and only a render answers
    /// that.
    ///
    /// ```sh
    /// cargo test -p crowbar-app an_inset_shadow -- --ignored --test-threads=1 --nocapture
    /// ```
    /// **Does hovering a row actually change it?**
    ///
    /// Written after reporting hover states as "done and measured" on the
    /// strength of a clean build and an unchanged *resting* render. Neither is
    /// evidence about hover: a resting diff is precisely the instrument that
    /// cannot see one. This moves a pointer and reads the pixels under it.
    ///
    /// ```sh
    /// cargo test -p crowbar-app hovering_a_row -- --ignored --test-threads=1 --nocapture
    /// ```
    /// Control for [`hovering_a_row_changes_what_is_painted`]: does a bare
    /// `div().hover(...)` change colour under a simulated pointer at all?
    ///
    /// If this passes and the row test fails, the fault is in the surfaces. If
    /// both fail, `simulate_mouse_move` is not a usable stand-in for a pointer
    /// and the instrument is wrong, not the app.
    #[test]
    #[ignore = "drives Metal, which is main-thread only"]
    fn a_bare_div_changes_colour_under_a_simulated_pointer() {
        use crowbar_ui::gpui::{
            AppContext as _, HeadlessAppContext, InteractiveElement as _, ParentElement as _,
            Render, StatefulInteractiveElement as _, Styled as _, div, point, px, size,
        };

        struct Probe;
        impl Render for Probe {
            fn render(
                &mut self,
                _window: &mut crowbar_ui::gpui::Window,
                _cx: &mut crowbar_ui::gpui::Context<Self>,
            ) -> impl crowbar_ui::gpui::IntoElement {
                let fill = crowbar_ui::theme::Color::BLACK
                    .mix(88.0, crowbar_ui::theme::Color::WHITE)
                    .value();
                let lit = crowbar_ui::theme::Color::WHITE.value();
                div()
                    .size_full()
                    .bg(crowbar_ui::theme::Color::BLACK.value())
                    // Left box: stateless, exactly how the surfaces build.
                    .child(
                        div()
                            .absolute()
                            .top(px(10.0))
                            .left(px(5.0))
                            .w(px(40.0))
                            .h(px(40.0))
                            .bg(fill)
                            .hover(move |style| style.bg(lit)),
                    )
                    // Right box: identical but **stateful**. If only this one
                    // lights up, gpui wants an element id for interaction
                    // states and the surfaces have to carry one.
                    .child(
                        div()
                            .id("stateful-probe")
                            .absolute()
                            .top(px(10.0))
                            .left(px(55.0))
                            .w(px(40.0))
                            .h(px(40.0))
                            .bg(fill)
                            .hover(move |style| style.bg(lit)),
                    )
            }
        }

        let platform = gpui_platform::current_platform(true);
        let mut cx = HeadlessAppContext::with_platform(
            platform.text_system(),
            std::sync::Arc::new(crate::Assets),
            gpui_platform::current_headless_renderer,
        );
        let window = cx
            .open_window(size(px(100.0), px(60.0)), |_window, cx| cx.new(|_| Probe))
            .expect("a headless window opens");
        cx.run_until_parked();

        let resting = cx
            .capture_screenshot(window.into())
            .expect("a resting frame");
        cx.update_window(window.into(), |_view, window, cx| {
            window.simulate_mouse_move(point(px(75.0), px(30.0)), cx);
        })
        .expect("the pointer moves");
        cx.update_window(window.into(), |_view, window, _cx| {
            println!(
                "  after the move, window.mouse_position() = {:?}",
                window.mouse_position()
            );
        })
        .expect("read back");
        cx.run_until_parked();
        cx.update_window(window.into(), |_view, window, _cx| {
            println!(
                "  after a draw,    window.mouse_position() = {:?}",
                window.mouse_position()
            );
        })
        .expect("read back");
        // An extra draw. gpui computes `mouse_hit_test` at the *end* of a
        // frame, so the frame the move itself draws still hit-tests the old
        // position; the hover only lands on the frame after it.
        // **No forced draw.** This is the running app's situation: gpui's
        // `dispatch_mouse_event` updates the hit test and resets the cursor but
        // does not `refresh()`, so a hover only reaches the screen if something
        // asks for a repaint. Stateful elements register a listener that does;
        // stateless ones have nothing to. Forcing `window.draw` here would hide
        // exactly the difference this test exists to show.
        cx.run_until_parked();
        let hovered = cx
            .capture_screenshot(window.into())
            .expect("a hovered frame");

        let scale = resting.width() / 100;
        let at = |image: &image::RgbaImage, x: u32| image.get_pixel(x * scale, 30 * scale).0;
        // The pointer is parked at (25, 30): inside the stateless box only.
        println!(
            "stateless  resting {:?}  hovered {:?}   (pointer is NOT over this one)",
            at(&resting, 25),
            at(&hovered, 25)
        );
        println!(
            "stateful   resting {:?}  hovered {:?}   <- pointer is here",
            at(&resting, 75),
            at(&hovered, 75)
        );
        assert_ne!(
            at(&resting, 75),
            at(&hovered, 75),
            "a stateful div did not repaint its hover without a forced draw"
        );
    }

    /// **Does clicking a tab change the active tab?**
    ///
    /// Behaviour, not pixels: a press is only real if it moves the store. The
    /// window is built exactly as the shipping one is, through
    /// `Sidebar::build`, so the sink under test is the app's own dispatch and
    /// not a stand-in.
    ///
    /// ```sh
    /// cargo test -p crowbar-app clicking_a_tab -- --ignored --test-threads=1 --nocapture
    /// ```
    /// **Does the indicator slide, or jump?**
    ///
    /// Presses the Chats tab, then advances the clock through the 200ms
    /// transition, reading the indicator's left edge out of each frame. A jump
    /// shows two positions; a slide shows a monotone sweep between them.
    ///
    /// ```sh
    /// cargo test -p crowbar-app the_indicator_slides -- --ignored --test-threads=1 --nocapture
    /// ```
    #[test]
    #[ignore = "drives Metal, which is main-thread only"]
    fn the_indicator_slides_between_tabs_rather_than_jumping() {
        use super::{Backdrop, Shell, Sidebar, SidebarStore};
        use crowbar_ui::gpui::{
            AppContext as _, HeadlessAppContext, Modifiers, MouseButton, MouseDownEvent,
            MouseUpEvent, PlatformInput, point, px, size,
        };

        let platform = gpui_platform::current_platform(true);
        let mut cx = HeadlessAppContext::with_platform(
            platform.text_system(),
            std::sync::Arc::new(crate::Assets),
            gpui_platform::current_headless_renderer,
        );
        cx.update(|cx| {
            crate::load_ui_font(cx);
            crate::load_ui_mono_font(cx);
            gpui_component::init(cx);
        });
        let store = cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| fixture_seed()(store, cx));
            store
        });
        let window = cx
            .open_window(size(px(294.0), px(400.0)), |_window, cx| {
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

        // The indicator is the one opaque dark box on the tab strip: find its
        // left edge by scanning the strip's middle row for `rgb(31, 31, 30)`.
        let indicator_left = |image: &image::RgbaImage| -> Option<u32> {
            let scale = image.width() / 294;
            let row = 119 * scale;
            (0..image.width())
                .find(|x| {
                    let pixel = image.get_pixel(*x, row).0;
                    pixel[0] == 31 && pixel[1] == 31 && pixel[2] == 30
                })
                .map(|x| x / scale)
        };

        let resting = cx
            .capture_screenshot(window.into())
            .expect("a resting frame");
        let start = indicator_left(&resting);

        let target = point(px(147.0), px(119.0));
        cx.update_window(window.into(), |_view, window, cx| {
            window.simulate_mouse_move(target, cx);
            let _ = window.draw(cx);
            window.dispatch_event(
                PlatformInput::MouseDown(MouseDownEvent {
                    button: MouseButton::Left,
                    position: target,
                    modifiers: Modifiers::default(),
                    click_count: 1,
                    first_mouse: false,
                }),
                cx,
            );
            window.dispatch_event(
                PlatformInput::MouseUp(MouseUpEvent {
                    button: MouseButton::Left,
                    position: target,
                    modifiers: Modifiers::default(),
                    click_count: 1,
                }),
                cx,
            );
        })
        .expect("the press is delivered");
        cx.run_until_parked();

        let mut positions = Vec::new();
        for step in 0..8 {
            cx.update_window(window.into(), |_view, window, cx| {
                let _ = window.draw(cx);
            })
            .expect("a frame");
            cx.advance_clock(std::time::Duration::from_millis(25));
            cx.run_until_parked();
            let frame = cx.capture_screenshot(window.into()).expect("a frame");
            let left = indicator_left(&frame);
            println!("  t={:>3}ms  indicator left = {left:?}", step * 25);
            if let Some(left) = left {
                positions.push(left);
            }
        }

        println!("resting left = {start:?}, sweep = {positions:?}");
        let distinct: std::collections::BTreeSet<u32> = positions.iter().copied().collect();
        assert!(
            distinct.len() > 2,
            "the indicator occupied only {} position(s) across the transition — it is \
             jumping, not sliding: {positions:?}",
            distinct.len()
        );
    }

    #[test]
    #[ignore = "drives Metal, which is main-thread only"]
    fn clicking_a_tab_moves_the_active_tab() {
        use super::{Backdrop, Shell, Sidebar, SidebarStore};
        use crowbar_ui::gpui::{
            AppContext as _, HeadlessAppContext, Modifiers, MouseButton, MouseDownEvent,
            MouseUpEvent, PlatformInput, point, px, size,
        };

        let platform = gpui_platform::current_platform(true);
        let mut cx = HeadlessAppContext::with_platform(
            platform.text_system(),
            std::sync::Arc::new(crate::Assets),
            gpui_platform::current_headless_renderer,
        );
        cx.update(|cx| {
            crate::load_ui_font(cx);
            crate::load_ui_mono_font(cx);
            gpui_component::init(cx);
        });
        let store = cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| fixture_seed()(store, cx));
            store
        });
        let window = cx
            .open_window(size(px(294.0), px(400.0)), |_window, cx| {
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

        let before = cx.update(|cx| store.read(cx).active_tab());
        println!("active tab before: {before:?}");

        // The Chats tab: x 102..192, y 103..135 — its centre.
        let target = point(px(147.0), px(119.0));
        cx.update_window(window.into(), |_view, window, cx| {
            window.simulate_mouse_move(target, cx);
            let _ = window.draw(cx);
            window.dispatch_event(
                PlatformInput::MouseDown(MouseDownEvent {
                    button: MouseButton::Left,
                    position: target,
                    modifiers: Modifiers::default(),
                    click_count: 1,
                    first_mouse: false,
                }),
                cx,
            );
            window.dispatch_event(
                PlatformInput::MouseUp(MouseUpEvent {
                    button: MouseButton::Left,
                    position: target,
                    modifiers: Modifiers::default(),
                    click_count: 1,
                }),
                cx,
            );
        })
        .expect("the press is delivered");
        cx.run_until_parked();

        let after = cx.update(|cx| store.read(cx).active_tab());
        println!("active tab after:  {after:?}");
        assert_ne!(
            before, after,
            "pressing the Chats tab left the active tab where it was — the press \
             is not reaching the store"
        );
    }

    #[test]
    #[ignore = "drives Metal, which is main-thread only"]
    fn hovering_a_row_changes_what_is_painted() {
        use super::{Backdrop, Shell, Sidebar, SidebarStore};
        use crowbar_ui::gpui::{AppContext as _, HeadlessAppContext, point, px, size};

        let platform = gpui_platform::current_platform(true);
        let mut cx = HeadlessAppContext::with_platform(
            platform.text_system(),
            std::sync::Arc::new(crate::Assets),
            gpui_platform::current_headless_renderer,
        );
        cx.update(|cx| {
            crate::load_ui_font(cx);
            crate::load_ui_mono_font(cx);
            gpui_component::init(cx);
        });
        let store = cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| fixture_seed()(store, cx));
            store
        });
        let window = cx
            .open_window(size(px(294.0), px(400.0)), |_window, cx| {
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

        // A workspace row: logical y 223..259, x 20..288. Sample clear of its
        // label and of its trailing button.
        let (probe_x, probe_y): (u32, u32) = (200, 241);
        let resting = cx
            .capture_screenshot(window.into())
            .expect("a frame before the pointer moves");

        cx.update_window(window.into(), |_view, window, cx| {
            let at = |value: u32| px(f32::from(u16::try_from(value).unwrap_or(0)));
            window.simulate_mouse_move(point(at(probe_x), at(probe_y)), cx);
        })
        .expect("the pointer moves");
        cx.run_until_parked();

        // Captured repeatedly: gpui computes `mouse_hit_test` *after* painting
        // a frame, so the first frame drawn after a move still hit-tests the
        // previous pointer position. If hover needs a second frame to appear,
        // that is a fact worth seeing rather than a reason to give up.
        let mut frames = Vec::new();
        for _ in 0..3 {
            // **No forced draw.** The shell asks for the repaint itself, so if
            // hover only appears when this test draws by hand, the app does not
            // have it. See `Shell::render`'s `on_mouse_move`.
            cx.run_until_parked();
            frames.push(
                cx.capture_screenshot(window.into())
                    .expect("a frame while the pointer rests on the row"),
            );
        }
        let hovered = frames.last().cloned().expect("three frames were captured");

        let scale = resting.width() / 294;
        let at = |image: &image::RgbaImage| image.get_pixel(probe_x * scale, probe_y * scale).0;
        let (before, after) = (at(&resting), at(&hovered));
        for (index, frame) in frames.iter().enumerate() {
            println!("  frame {index} after the move: {:?}", at(frame));
        }
        println!("row pixel  resting {before:?}  hovered {after:?}");

        assert_ne!(
            before, after,
            "the row painted identically with the pointer on it — `hover:bg-accent` \
             is not reaching the screen"
        );
    }

    #[test]
    #[ignore = "drives Metal, which is main-thread only"]
    fn an_inset_shadow_paints_a_hairline_inside_the_top_edge() {
        use crowbar_ui::gpui::{
            AppContext as _, BoxShadow, HeadlessAppContext, ParentElement as _, Render,
            Styled as _, div, point, px, size,
        };

        /// The border width the probe draws, in whole logical pixels. An
        /// integer because it is also used as a row *index*, and a float
        /// round-tripped through `as u32` is a truncation the lint is right
        /// to refuse.
        const BORDER: u32 = 1;

        /// [`BORDER`] as a `Pixels`, in one place.
        fn border() -> crowbar_ui::gpui::Pixels {
            px(f32::from(u16::try_from(BORDER).unwrap_or(1)))
        }

        struct Probe;
        impl Render for Probe {
            fn render(
                &mut self,
                _window: &mut crowbar_ui::gpui::Window,
                _cx: &mut crowbar_ui::gpui::Context<Self>,
            ) -> impl crowbar_ui::gpui::IntoElement {
                div()
                    .size_full()
                    .bg(crowbar_ui::theme::Color::BLACK.value())
                    .child(
                        div()
                            .absolute()
                            .top(px(10.0))
                            .left(px(10.0))
                            .w(px(80.0))
                            .h(px(40.0))
                            // Dark, like the real active row: a 16% white highlight over a light
                            // fill is only ~8 levels, which is too small to assert on.
                            .bg(crowbar_ui::theme::Color::BLACK
                                .mix(88.0, crowbar_ui::theme::Color::WHITE)
                                .value())
                            // **Bordered**, which is the case that failed: gpui
                            // paints the border after the inset shadows, so a
                            // highlight offset by less than the border width is
                            // painted and then covered. `elevation::raised` takes
                            // the border width for exactly this reason, and this
                            // probe reproduces the geometry it has to survive.
                            .border(border())
                            .border_color(
                                crowbar_ui::theme::Color::BLACK
                                    .mix(88.0, crowbar_ui::theme::Color::WHITE)
                                    .value(),
                            )
                            .shadow(crowbar_ui::elevation::raised(border())),
                    )
            }
        }

        let platform = gpui_platform::current_platform(true);
        let mut cx = HeadlessAppContext::with_platform(
            platform.text_system(),
            std::sync::Arc::new(crate::Assets),
            gpui_platform::current_headless_renderer,
        );
        let window = cx
            .open_window(size(px(100.0), px(60.0)), |_window, cx| cx.new(|_| Probe))
            .expect("a headless window opens");
        cx.run_until_parked();
        let image = cx
            .capture_screenshot(window.into())
            .expect("the headless renderer produced a frame");

        let scale = image.width() / 100;
        let column = 50 * scale;
        let sample = |y: u32| image.get_pixel(column, y).0;
        let rows: Vec<[u8; 4]> = (9 * scale..14 * scale).map(sample).collect();
        println!("rows 9..14 (logical) at x=50: {rows:?}");

        // The box's top edge is at logical 10; logical 10 is its border, so the
        // highlight must land at logical 11, *inside* it — which is where CSS
        // puts an inset shadow, and where this rendered nothing until the
        // offset absorbed the border width.
        //
        // Compared against the box's **own background**, not against the
        // border. Comparing against the border is the vacuous version of this
        // test: with the highlight painted under a black border, the row
        // inside it is simply the box's fill, which is still far brighter than
        // black — so it passed with the bug in place. Asserted here by
        // mutation: dropping `border_width` from the offset must fail this.
        let fill = sample(20 * scale)[0];
        let highlight = sample((10 + BORDER) * scale)[0];
        assert!(
            highlight > fill + 20,
            "no inset hairline inside the border: the row inside the border read {highlight} \
             and the box's own fill reads {fill}, so nothing was painted there. The highlight \
             is landing under the border. rows = {rows:?}"
        );
    }

    #[test]
    #[ignore = "drives Metal, which is main-thread only; see the doc comment"]
    fn write_the_sidebar_png() {
        let out = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../target/sidebar.png");
        // 294x1119 — the Tauri window's own logical size when the reference
        // was captured. The comparison is pixel-for-pixel over the same
        // columns and rows, so the two images have to be the same shape.
        render_sidebar_png(px(294.0), px(1119.0), fixture_seed(), &out);
        assert!(out.exists(), "no image was written to {}", out.display());
    }
}
