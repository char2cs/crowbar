//! The sidebar entity: frame, header, space switcher, mode bar and the three panels.

use std::rc::Rc;

use gpui::{AnyView, AppContext as _, Context, Entity, EventEmitter, FocusHandle};
use ui::{CubicBezier, Glide, ToastQueue};

use crate::adapters::system_clock::system_clock;
use crate::clock::Clock;
use crate::command_sink::CommandSink;
use crate::drag_state::DragState;
use crate::file_search::FileSearch;
use crate::icon_editor::IconEditor;
use crate::inline_edit::InlineEdit;
use crate::merge_draft::MergeDraft;
use crate::mode::Mode;
use crate::open_menu::OpenMenu;
use crate::panel::Panel;
use crate::peek::Peek;
use crate::preferences::Preferences;
use crate::question::Question;
use crate::resizing::Resizing;
use crate::row_bounds::RowBounds;
use crate::row_id::RowId;
use crate::scrolls::Scrolls;
use crate::sidebar_data::SidebarData;
use crate::sidebar_event::SidebarEvent;
use crate::space_draft::SpaceDraft;
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
    pub(crate) clock: Rc<dyn Clock>,
    pub(crate) scrolls: Scrolls,
    pub(crate) handle_focus: FocusHandle,
    pub(crate) files_focus: FocusHandle,
    pub(crate) space: Option<SpaceDraft>,
    pub(crate) icon_editor: Entity<IconEditor>,
    pub(crate) question: Option<Question>,
    pub(crate) merge: MergeDraft,
    pub(crate) search: Option<FileSearch>,
    pub(crate) toasts: Entity<ToastQueue>,
    pub(crate) drag: DragState,
    pub(crate) row_bounds: RowBounds,
}

impl EventEmitter<SidebarEvent> for Sidebar {}

impl Sidebar {
    /// A sidebar over `data` that sends commands to `sink` and frames `content`, restoring the
    /// width, mode and folds from `preferences`.
    pub fn new(
        data: Rc<dyn SidebarData>,
        sink: Rc<dyn CommandSink>,
        content: AnyView,
        setup: (Preferences, &mut Context<'_, Self>),
    ) -> Self {
        let (preferences, cx) = setup;
        let dock = preferences.panel().side();
        let clock: Rc<dyn Clock> = Rc::new(system_clock());
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
            peek: Peek::new(Rc::clone(&clock)),
            clock,
            scrolls: Scrolls::default(),
            handle_focus: cx.focus_handle(),
            files_focus: cx.focus_handle(),
            space: None,
            icon_editor: Self::space_icon_editor(cx),
            question: None,
            merge: MergeDraft::new(cx, Self::commit_changes),
            search: None,
            toasts: cx.new(|_| ToastQueue::new(dock.docked_toasts())),
            drag: DragState::default(),
            row_bounds: RowBounds::default(),
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
        self.tree.removals.ask_to_discard(row, at_risk);
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
        self.toasts.update(cx, |queue, cx| {
            queue.set_placement(side.docked_toasts(), cx);
        });
        Self::changed(cx);
    }

    pub(crate) fn changed(cx: &mut Context<'_, Self>) {
        cx.emit(SidebarEvent::PreferencesChanged);
        cx.notify();
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
            this.tree.removals.confirmation().map(Confirmation::title)
        });
        assert!(title.is_some_and(|text| text.ends_with("and lose work?")));
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
