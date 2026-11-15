//! The sidebar entity: frame, header, space switcher, mode bar and the three panels.

use std::collections::HashMap;
use std::rc::Rc;

use gpui::{AnyView, Context, EventEmitter, FocusHandle, ScrollHandle};
use ui::{CubicBezier, Glide};

use crate::command_sink::CommandSink;
use crate::drag_state::DragState;
use crate::inline_edit::InlineEdit;
use crate::mode::Mode;
use crate::open_menu::OpenMenu;
use crate::panel::Panel;
use crate::peek::Peek;
use crate::preferences::Preferences;
use crate::project_id::ProjectId;
use crate::resizing::Resizing;
use crate::row_id::RowId;
use crate::sidebar_data::SidebarData;
use crate::sidebar_event::SidebarEvent;
use crate::tree_state::TreeState;
use crate::work_at_risk::WorkAtRisk;

const CAROUSEL_SPAN: std::time::Duration = std::time::Duration::from_millis(300);

/// The sidebar and the frame around it: it lays out the sidebar column, the resize handle and
/// `content`, owns width and mode, and renders the data the shell provides.
pub struct Sidebar {
    pub(crate) data: Rc<dyn SidebarData>,
    pub(crate) sink: Rc<dyn CommandSink>,
    pub(crate) content: AnyView,
    pub(crate) preferences: Preferences,
    pub(crate) panel: Panel,
    pub(crate) resizing: Option<Resizing>,
    pub(crate) mode: Mode,
    pub(crate) tree: TreeState,
    pub(crate) inline: Option<InlineEdit>,
    pub(crate) menu: Option<OpenMenu>,
    pub(crate) carousel: Glide,
    pub(crate) peek: Peek,
    pub(crate) scrolls: HashMap<ProjectId, ScrollHandle>,
    pub(crate) files_scroll: ScrollHandle,
    pub(crate) changes_scroll: ScrollHandle,
    pub(crate) handle_focus: FocusHandle,
    pub(crate) creating_space: bool,
    pub(crate) drag: DragState,
}

impl EventEmitter<SidebarEvent> for Sidebar {}

impl Sidebar {
    /// A sidebar over `data` that sends commands to `sink` and frames `content`, restoring the
    /// width, mode and folds from `preferences`.
    pub fn new(
        data: Rc<dyn SidebarData>,
        sink: Rc<dyn CommandSink>,
        content: AnyView,
        (preferences, cx): (Preferences, &mut Context<'_, Self>),
    ) -> Self {
        Self {
            data,
            sink,
            content,
            panel: preferences.panel(),
            mode: preferences.mode,
            tree: preferences.tree(),
            preferences,
            resizing: None,
            inline: None,
            menu: None,
            carousel: Glide::resting(0.0, CAROUSEL_SPAN, CubicBezier::EASE),
            peek: Peek::default(),
            scrolls: HashMap::new(),
            files_scroll: ScrollHandle::new(),
            changes_scroll: ScrollHandle::new(),
            handle_focus: cx.focus_handle(),
            creating_space: false,
            drag: DragState::default(),
        }
    }

    /// The preferences matching the current state.
    pub fn preferences(&self) -> Preferences {
        self.preferences.updated(self.panel, self.mode, &self.tree)
    }

    /// Opens or closes the sidebar (Mod+B, the header button).
    pub fn toggle(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.panel.toggle();
        self.peek.slide_to(0.0, ui::Motion::of(cx));
        Self::changed(cx);
    }

    /// Asks again before discarding `row`, naming the work a refused removal would lose.
    pub fn ask_to_discard(
        &mut self,
        row: &RowId,
        at_risk: Vec<WorkAtRisk>,
        cx: &mut Context<'_, Self>,
    ) {
        self.tree.ask_to_discard(row, at_risk);
        cx.notify();
    }

    /// Chooses a mode (Mod+1, Mod+2, Mod+3).
    pub fn set_mode(
        &mut self,
        mode: Mode,
        cx: &mut Context<'_, Self>,
    ) {
        if !mode.is_available(self.data.focus()) {
            return;
        }
        self.mode = mode;
        Self::changed(cx);
    }

    /// Docks the sidebar to the other window edge.
    pub fn dock_to(
        &mut self,
        side: crate::dock_side::DockSide,
        cx: &mut Context<'_, Self>,
    ) {
        self.panel.dock_to(side);
        Self::changed(cx);
    }

    pub(crate) fn changed(cx: &mut Context<'_, Self>) {
        cx.emit(SidebarEvent::PreferencesChanged);
        cx.notify();
    }

    pub(crate) fn scroll_for(
        &mut self,
        project: &ProjectId,
    ) -> ScrollHandle {
        self.scrolls.entry(project.clone()).or_default().clone()
    }
}

impl std::fmt::Debug for Sidebar {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Sidebar")
            .field("panel", &self.panel)
            .field("mode", &self.mode)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::confirmation::Confirmation;
    use crate::dock_side::DockSide;
    use crate::intent::Intent;
    use crate::test_support::prelude::*;

    fn bump(counter: &Rc<std::cell::Cell<u32>>) {
        counter.set(counter.get() + 1);
    }

    #[gpui::test]
    fn changes_to_the_geometry_are_reported_as_preferences(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let events = Rc::new(std::cell::Cell::new(0));
        let seen = Rc::clone(&events);
        let _subscription =
            cx.update(|_, cx| cx.subscribe(&sidebar, move |_, _: &SidebarEvent, _| bump(&seen)));
        sidebar.update(cx, |this, cx| this.dock_to(DockSide::Right, cx));
        assert_eq!(events.get(), 1);
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.preferences().side),
            DockSide::Right
        );
    }

    #[gpui::test]
    fn a_refused_removal_asks_again_naming_the_loss(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let risk = WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: 2,
            unmerged_commits: 0,
        };
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::RequestRemoval(RowId::from("chat")), window, cx);
            this.ask_to_discard(&RowId::from("chat"), vec![risk], cx);
        });
        let title = sidebar.read_with(cx, |this, _| {
            this.tree.confirmation().map(Confirmation::title)
        });
        assert!(title.is_some_and(|text| text.ends_with("and lose work?")));
    }

    #[gpui::test]
    fn each_space_keeps_its_own_scroll_position(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        let (a, b) = sidebar.update(cx, |this, _| {
            (
                this.scroll_for(&ProjectId::from("inv")),
                this.scroll_for(&ProjectId::from("other")),
            )
        });
        a.set_offset(point(px(0.0), px(-10.0)));
        assert_ne!(a.offset(), b.offset());
    }

    #[gpui::test]
    fn toggling_twice_restores_the_sidebar(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let before = sidebar.read_with(cx, |this, _| this.panel.visibility());
        sidebar.update(cx, |this, cx| {
            this.toggle(cx);
            this.toggle(cx);
        });
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.panel.visibility()),
            before
        );
    }
}
