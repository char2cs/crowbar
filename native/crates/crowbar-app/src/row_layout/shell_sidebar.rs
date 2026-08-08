//! What the assembled sidebar's layout actually resolves to.
//!
//! Slice 1a. The rest of `row_layout/` measures one surface in isolation,
//! which is what the retired method needed; this one measures the
//! **composition**, because that is where the archive's finding lives — ten
//! surfaces held a PASS verdict while the assembled app was a bare text list,
//! and nothing in the corpus could see it.
//!
//! It exists because the app rendered nothing on screen while its store held
//! four repos of real daemon data. Screen Recording is not granted on this
//! machine, so pixels were unavailable; this reads the geometry back instead,
//! which is the same instrument the Phase 1 gate used and needs no permission.

use crowbar_core::sidebar::collapse::Collapsed;
use crowbar_core::sidebar::hierarchy::build_workspace_tree;
use crowbar_core::sidebar::tree::{AvatarSource, SidebarRepo, SidebarWorkspace};
use crowbar_ui::surfaces::repo::repo_section::RepoSection;
use crowbar_ui::theme::Theme;
use crowbar_ui::ui_sans_font;
use gpui::{
    Context, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext, Window, div, px,
    size,
};

use super::lay_out;
use crate::driver_anchors::{DriverAnchors, fold_text_halves};
use crate::shell::model;

fn workspace(id: &str) -> SidebarWorkspace {
    SidebarWorkspace {
        id: id.to_string(),
        branch: format!("branch-{id}"),
        parent_id: None,
        status: None,
        added: 3,
        deleted: 1,
        working: false,
        can_merge_locally: false,
        merge_conflicts: false,
        parent_branch: None,
        pr_url: None,
        last_error: String::new(),
        held_by_path: String::new(),
        local_path: Some(format!("/w/{id}")),
    }
}

fn repo() -> SidebarRepo {
    SidebarRepo {
        id: "r1".to_string(),
        project_id: "p1".to_string(),
        name: "crowbar".to_string(),
        avatar_label: "C".to_string(),
        avatar_color: "avatar-slate".to_string(),
        avatar_source: AvatarSource::Initials,
        workspaces: vec![workspace("w1"), workspace("w2")],
        default_workspace_id: Some("w-default".to_string()),
        default_branch: Some("main".to_string()),
        default_working: false,
        default_status: None,
        local_path: None,
    }
}

/// Renders one repo section inside a sidebar-width column — the simplest
/// nesting that could work, as a control.
struct Stage {
    section: RepoSection,
}

impl Render for Stage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::DARK;
        div()
            .font(ui_sans_font(&theme))
            .w(px(294.0))
            .h_full()
            .flex()
            .flex_col()
            .child(self.section.render(&theme, &DriverAnchors, &crowbar_ui::Inert))
    }
}

/// Renders it through **the app's own nesting**: the carousel strip, a
/// full-width panel, and the workspace column inside it.
///
/// The control above and this differ in exactly that chain, which is what
/// makes the pair a diagnosis rather than an observation.
struct NestedStage {
    section: RepoSection,
}

impl Render for NestedStage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::DARK;
        let column = div()
            .flex()
            .flex_col()
            .flex_1()
            .min_h(px(0.0))
            .overflow_hidden()
            .child(self.section.render(&theme, &DriverAnchors, &crowbar_ui::Inert));

        let panel = div()
            .w(px(294.0))
            .flex_shrink_0()
            .flex()
            .flex_col()
            .overflow_hidden()
            .child(column);

        let strip = div().flex().flex_1().min_h(px(0.0)).child(panel);

        div()
            .font(ui_sans_font(&theme))
            .relative()
            .flex()
            .flex_col()
            .h_full()
            .w(px(294.0))
            .overflow_hidden()
            .child(strip)
    }
}

/// The sidebar's own default width, and a window tall enough that nothing is
/// cut by it — a surface clipped by the window would report bounds that say
/// more about the harness than about the layout.
fn stage(cx: &mut TestAppContext) -> Vec<crowbar_driver::RawAnchor> {
    let section = model::repo_section(
        &repo(),
        &build_workspace_tree(&repo().workspaces),
        None,
        &Collapsed::new(),
        &Theme::DARK,
    );
    let (_registry, anchors) = lay_out(cx, size(px(294.0), px(800.0)), |_, _| Stage { section });
    fold_text_halves(anchors)
}

/// The same section, laid out through the app's own nesting.
fn nested(cx: &mut TestAppContext) -> Vec<crowbar_driver::RawAnchor> {
    let section = model::repo_section(
        &repo(),
        &build_workspace_tree(&repo().workspaces),
        None,
        &Collapsed::new(),
        &Theme::DARK,
    );
    let (_registry, anchors) =
        lay_out(cx, size(px(294.0), px(800.0)), |_, _| NestedStage { section });
    fold_text_halves(anchors)
}

/// The diagnosis: the same section, the same window, the only difference
/// being the carousel chain the app wraps it in.
#[gpui::test]
fn the_apps_own_nesting_keeps_the_section_visible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let anchors = nested(cx);

    for anchor in &anchors {
        eprintln!(
            "NESTED {:<34} x={:>7.1} y={:>7.1} w={:>7.1} h={:>7.1} visible={}",
            anchor.id.as_ref(),
            f32::from(anchor.bounds.origin.x),
            f32::from(anchor.bounds.origin.y),
            f32::from(anchor.bounds.size.width),
            f32::from(anchor.bounds.size.height),
            anchor.visible,
        );
    }

    let section = anchors
        .iter()
        .find(|anchor| anchor.id.as_ref() == "repo-section")
        .expect("the section is recorded");

    assert!(
        f32::from(section.bounds.size.height) > 0.0,
        "the carousel chain collapsed the section to zero height"
    );
}

/// **The regression this file was written for.** The composed sidebar has to
/// resolve to real geometry — a repo section that lays out at zero height
/// renders nothing, and the app looks broken while every one of its parts
/// holds a passing verdict.
#[gpui::test]
fn the_composed_repo_section_has_real_geometry(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let anchors = stage(cx);

    assert!(
        !anchors.is_empty(),
        "the composition recorded no anchors at all"
    );

    for anchor in &anchors {
        eprintln!(
            "ANCHOR {:<34} x={:>7.1} y={:>7.1} w={:>7.1} h={:>7.1} visible={}",
            anchor.id.as_ref(),
            f32::from(anchor.bounds.origin.x),
            f32::from(anchor.bounds.origin.y),
            f32::from(anchor.bounds.size.width),
            f32::from(anchor.bounds.size.height),
            anchor.visible,
        );
    }

    let painted = anchors
        .iter()
        .filter(|anchor| {
            anchor.visible
                && f32::from(anchor.bounds.size.width) > 0.0
                && f32::from(anchor.bounds.size.height) > 0.0
        })
        .count();

    assert!(
        painted > 0,
        "every anchor in the composed section is invisible or zero-sized"
    );
}
