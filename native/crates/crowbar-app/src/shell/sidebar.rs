//! The sidebar, assembled.
//!
//! Slice 1a's deliverable and the first thing in this port to compose the
//! design system rather than measure one component of it at a time.
//!
//! # The carousel is built here, not reused
//!
//! `crowbar_ui::surfaces::sidebar::sidebar_carousel` is a **geometry-only**
//! surface: its `panel()` renders an empty box per tab, because the retired
//! method measured the strip's boxes and never its contents. The mechanism —
//! four full-width panels on one horizontally-offset strip, snapping to a
//! panel boundary — is reproduced here with the real workspace tree in panel
//! 0 and empty placeholders in the other three, whose contents are slices 3,
//! 4 and 5.
//!
//! The offset arithmetic is not open-coded: `Tab::offset` and
//! `Tab::at_offset` own it in `crowbar-core`, where the collapsed-sidebar case
//! that silently moved Files to Chats is tested.

use crowbar_core::sidebar::tabs::Tab;
use crowbar_state::SidebarStore;
use crowbar_ui::Unanchored;
use crowbar_ui::gpui::{
    AnyElement, App, AppContext as _, Context, Entity, IntoElement, ParentElement as _, Pixels,
    Render, Styled as _, Window, div, px,
};
use crowbar_ui::surfaces::sidebar::sidebar_tab_bar::SidebarTabBar;
use crowbar_ui::surfaces::workspace::workspace_tree::WorkspaceTree;
use crowbar_ui::theme::Theme;
use crowbar_ui::{ActionId, ActionSink, Dispatch};

use super::model;

/// Action parts this surface dispatches on. String-valued because
/// [`ActionId`] is `crowbar-ui`'s type and the design system does not model
/// what a part means — see its own module docs.
const PART_ROW: &str = "workspace-row";
const PART_REPO: &str = "repo-header";
const PART_TAB: &str = "sidebar-tab";

/// The sidebar view.
pub struct Sidebar {
    store: Entity<SidebarStore>,
    theme: Theme,
}

impl Sidebar {
    /// Build the sidebar over `store`.
    pub fn build(store: &Entity<SidebarStore>, cx: &mut App) -> Entity<Self> {
        let store = store.clone();
        cx.new(|cx| {
            // Re-render whenever the store changes: every daemon frame, every
            // selection, every fold. `observe` rather than a manual notify
            // chain so a store mutation cannot forget to repaint.
            cx.observe(&store, |_, _, cx| cx.notify()).detach();
            Self {
                store,
                theme: Theme::DARK,
            }
        })
    }

    /// The sink that turns a press into a store mutation.
    ///
    /// Rebuilt per frame because it closes over the store handle; that is one
    /// `Rc` clone per render, not per element.
    fn actions(&self) -> impl ActionSink + use<> {
        let store = self.store.downgrade();
        Dispatch(move |id: &ActionId, _window: &mut Window, cx: &mut App| {
            let id = id.clone();
            let _ = store.update(cx, |store, cx| match id.part.as_ref() {
                PART_ROW => {
                    if let Some(scope) = store.scope_of(&id.subject).cloned() {
                        store.select_workspace(scope, cx);
                    }
                }
                PART_REPO => {
                    store.collapsed_mut().toggle_repo(&id.subject);
                    cx.notify();
                }
                PART_TAB => {
                    store.set_active_tab(Tab::from_str_or_default(&id.subject), cx);
                }
                _ => {}
            });
        })
    }

    /// The workspace panel: the real tree, from real daemon data.
    fn workspace_panel(&self, actions: &dyn ActionSink, cx: &Context<Self>) -> AnyElement {
        let store = self.store.read(cx);
        let active = store.active_workspace_id().map(str::to_owned);
        let collapsed = store.collapsed();

        let sections = store
            .repos()
            .iter()
            .map(|repo| {
                model::repo_section(
                    repo,
                    store.tree_for(&repo.id),
                    active.as_deref(),
                    collapsed,
                    &self.theme,
                )
            })
            .collect();

        let project_name = store
            .projects()
            .iter()
            .find(|project| Some(project.id.as_str()) == store.active_project_id())
            .map_or_else(String::new, |project| project.name.clone());

        let tree = WorkspaceTree {
            project_home: model::project_home_row(&project_name, false, false),
            sections,
            scroll_width: px(0.0),
            scroll_height: px(0.0),
        };

        // Hit targets are wrapped around the rendered rows rather than being
        // reached inside them: the surfaces are pure value types with no
        // handlers of their own, and `ActionSink` attaches to the boxes they
        // build. See `crowbar_ui::action`.
        let mut rows = div().flex().flex_col().w_full();
        for (index, repo) in store.repos().iter().enumerate() {
            let section = &tree.sections[index];
            rows = rows.child(
                actions
                    .clickable(
                        ActionId::new(PART_REPO, repo.id.clone()),
                        div().w_full().flex().flex_col(),
                    )
                    .child(section.render(&self.theme, &Unanchored)),
            );
        }

        div()
            .flex()
            .flex_col()
            .flex_1()
            .min_h(px(0.0))
            .overflow_hidden()
            .child(tree.project_home.render(&self.theme, &Unanchored))
            .child(rows)
            .into_any_element()
    }
}

/// A panel whose contents belong to a later slice.
fn blank_panel() -> AnyElement {
    div().flex().flex_col().flex_1().into_any_element()
}

impl Render for Sidebar {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let actions = self.actions();
        let store = self.store.read(cx);
        let active_tab = store.active_tab();
        let width = store.panel().preferred_width();

        let tab_bar = SidebarTabBar {
            active: active_tab.as_str().into(),
            include_git: true,
            column_width: px(width),
            viewport_breakpoint: crowbar_ui::surfaces::rows::git_status_row::Breakpoint::Sm,
        };

        let mut tabs = div().flex().w_full();
        for tab in Tab::ALL {
            tabs = tabs.child(actions.clickable(
                ActionId::new(PART_TAB, tab.as_str()),
                div().flex_1().h(px(28.0)),
            ));
        }

        // The strip: four full-width panels, offset so the active one is in
        // view. `Tab::offset` owns the arithmetic.
        let panel_width = px(width);
        let strip = div()
            .flex()
            .flex_1()
            .min_h(px(0.0))
            .ml(px(-active_tab.offset(width)))
            .child(panel(panel_width, self.workspace_panel(&actions, cx)))
            .child(panel(panel_width, blank_panel()))
            .child(panel(panel_width, blank_panel()))
            .child(panel(panel_width, blank_panel()));

        div()
            .relative()
            .flex()
            .flex_col()
            .h_full()
            .w(panel_width)
            .overflow_hidden()
            .child(tab_bar.render(&self.theme, &Unanchored))
            .child(tabs)
            .child(strip)
    }
}

/// One carousel panel: full width, never shrinking, so the strip's offset
/// arithmetic holds.
fn panel(width: Pixels, content: AnyElement) -> AnyElement {
    div()
        .w(width)
        .flex_shrink_0()
        .flex()
        .flex_col()
        .overflow_hidden()
        .child(content)
        .into_any_element()
}
