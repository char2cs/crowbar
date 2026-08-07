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
use crowbar_ui::AnchorSink;
use crowbar_ui::gpui::{
    AnyElement, App, AppContext as _, Context, Entity, IntoElement, ParentElement as _, Render,
    Styled as _, Window, div, px,
};
use crowbar_ui::primitives::keybinding::Platform;
use crowbar_ui::surfaces::context_pill::ContextPill;
use crowbar_ui::surfaces::rows::git_status_row::Breakpoint;
use crowbar_ui::surfaces::sidebar::sidebar_carousel::{SidebarCarousel, SidebarTab};
use crowbar_ui::surfaces::sidebar::sidebar_project_header::SidebarProjectHeader;
use crowbar_ui::surfaces::sidebar::sidebar_tab_bar::SidebarTabBar;
use crowbar_ui::surfaces::workspace::workspace_tree::WorkspaceTree;
use crowbar_ui::theme::Theme;
use crowbar_ui::{ActionId, ActionSink, Dispatch};
use std::rc::Rc;

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
    /// How this view's elements opt into an oracle snapshot.
    ///
    /// `Unanchored` on the shipping path and the driver's sink under
    /// `--inspect`, which is what lets the **real** window be read back
    /// without a screenshot. Injected rather than hardcoded because the
    /// alternative — a second view built for inspection — would mean the thing
    /// measured is not the thing shipped, which is the whole failure mode
    /// `crowbar_ui::anchor` exists to prevent.
    anchors: Rc<dyn AnchorSink>,
}

impl Sidebar {
    /// Build the sidebar over `store`.
    pub fn build(
        store: &Entity<SidebarStore>,
        anchors: Rc<dyn AnchorSink>,
        cx: &mut App,
    ) -> Entity<Self> {
        let store = store.clone();
        cx.new(|cx| {
            // Re-render whenever the store changes: every daemon frame, every
            // selection, every fold. `observe` rather than a manual notify
            // chain so a store mutation cannot forget to repaint.
            cx.observe(&store, |_, _, cx| cx.notify()).detach();
            Self {
                store,
                theme: Theme::DARK,
                anchors,
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

    /// The workspace panel: the ported [`WorkspaceTree`], carrying real
    /// daemon data.
    ///
    /// The tree surface owns the project-home row, the scroll area and the
    /// repo list — all three of which this file used to build by hand, badly.
    fn workspace_panel(&self, _actions: &dyn ActionSink, cx: &Context<Self>) -> AnyElement {
        let store = self.store.read(cx);
        let active = store.active_workspace_id().map(str::to_owned);
        let collapsed = store.collapsed();
        let width = px(store.panel().preferred_width());

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

        // Active on the project-home route, which is where the app sits until
        // a workspace is chosen. The captured reference paints this row
        // `#1f1f1eff` and this file drew it idle — which is, precisely, the
        // finding the archive recorded against the old shell: the component
        // holds a PASS in its `selected` cell and the app never produces that
        // cell. A home route is one with no repo.
        let on_home = store
            .active_scope()
            .is_none_or(|scope| scope.repo_id.is_empty());

        WorkspaceTree {
            project_home: model::project_home_row(&project_name, on_home, false),
            sections,
            // The scroll area's own extent. Zero here collapsed the whole list
            // — the surface authors a `w`/`h` on it, so it has to be the real
            // panel size, not a placeholder.
            scroll_width: width,
            scroll_height: px(0.0),
        }
        .render(&self.theme, &*self.anchors)
    }
}

impl Render for Sidebar {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let actions = self.actions();
        let store = self.store.read(cx);
        let width = px(store.panel().preferred_width());
        let project_name = store
            .projects()
            .iter()
            .find(|project| Some(project.id.as_str()) == store.active_project_id())
            .map_or_else(String::new, |project| project.name.clone());

        // `ide-shell.tsx`'s sidebar subtree, in its own order:
        //
        //     <div className="relative flex h-full flex-col overflow-hidden
        //                     bg-transparent select-none">
        //       {!hasNavScreen && <SidebarProjectHeader />}
        //       {!hasNavScreen && <SidebarTabBar />}
        //       <SidebarCarousel />
        //     </div>
        //
        // Every one of those is the ported surface, rendered by the ported
        // code — not a hand-written approximation of its CSS. That is the
        // whole correction: the containers were re-implemented here before,
        // and a re-implemented container is where a port stops looking like
        // the thing it ports.
        let header = SidebarProjectHeader {
            is_right: false,
            platform: Platform::Mac,
            // Nothing pushes a nav screen yet, so neither arrow is live. They
            // are rendered disabled rather than omitted, which is what the
            // reference does with an empty history.
            can_go_back: false,
            can_go_forward: false,
            toggle_id: "sidebar-project-header-toggle".into(),
            back_id: "sidebar-project-header-back".into(),
            forward_id: "sidebar-project-header-forward".into(),
            settings_id: "sidebar-project-header-settings".into(),
        };

        let tab_bar = SidebarTabBar {
            active: store.active_tab().as_str().into(),
            // `visibleTabs = isHomeRoute ? TABS.filter(t => t.tab !== 'git') :
            // TABS` — the git tab is hidden on the project-home route, and
            // this is **geometry**, not decoration: three tabs against four
            // moves every tab's width and the indicator's reach. The port's
            // own tab-bar docs call it "a real geometry-affecting axis".
            //
            // Caught by diffing the live app against the captured React
            // reference in `oracle/runs/p3.2-tabs/ref-tabs.json`: the
            // reference draws three tabs at 90px, this drew four at 67px,
            // because it was hardcoded `true`.
            //
            // A home route is one with no repo — `scope_url`'s own
            // `isHomeWorkspace` test, which is `repoId === ''`.
            include_git: store
                .active_scope()
                .is_some_and(|scope| !scope.repo_id.is_empty()),
            column_width: width,
            viewport_breakpoint: Breakpoint::Sm,
        };

        let carousel = SidebarCarousel {
            active: match store.active_tab() {
                Tab::Workspaces => SidebarTab::Workspaces,
                Tab::Chats => SidebarTab::Chats,
                Tab::Files => SidebarTab::Files,
                Tab::Git => SidebarTab::Git,
            },
            // Zero: the filler is what an empty panel draws, and panels 1..3
            // are empty until slices 3, 4 and 5. Panel 0 carries real content
            // and ignores it.
            panel_content_width: px(0.0),
        };

        let panels = vec![Some(self.workspace_panel(&actions, cx))];

        // NOT wrapped in `NavStack`. The reference does wrap the carousel in
        // one, and this did too for one build — and the tree vanished. The
        // stack's base layer sizes itself for the picture it was authored to
        // draw, so nesting a real, flex-sized carousel inside it collapses the
        // carousel to nothing. Re-adding it needs the base layer to grow with
        // its content, which is a change to that surface and not to this file.
        // Until then an empty stack contributes no visible box anyway: the
        // captured reference has `nav-stack` and `nav-stack-base` at exactly
        // the carousel's own bounds, painting nothing.

        // `<ContextPill />`, the row between the header and the tab bar. On the
        // project-home route the reference renders the Home variant; the
        // captured reference has it at 52.25px tall with a `#f5f5f51c`
        // trigger, and this file simply did not render it at all.
        let pill = ContextPill::Home {
            project_name: project_name.clone().into(),
            working: false,
        };

        div()
            .relative()
            .flex()
            .flex_col()
            .h_full()
            .w(width)
            .overflow_hidden()
            .child(header.render(&self.theme, &*self.anchors))
            .child(pill.render(&self.theme, &*self.anchors))
            .child(tab_bar.render(&self.theme, &*self.anchors))
            .child(carousel.render_with(&*self.anchors, panels))
    }
}
