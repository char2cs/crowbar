//! The sidebar entity: frame, header, space switcher, mode bar and the three panels.

use std::collections::HashMap;
use std::rc::Rc;

use gpui::{AnyView, Context, EventEmitter, FocusHandle, ScrollHandle};
use ui::{CubicBezier, Glide};

use crate::command_sink::CommandSink;
use crate::inline_edit::InlineEdit;
use crate::mode::Mode;
use crate::open_menu::OpenMenu;
use crate::panel::Panel;
use crate::peek::Peek;
use crate::preferences::Preferences;
use crate::project_id::ProjectId;
use crate::resizing::Resizing;
use crate::sidebar_data::SidebarData;
use crate::sidebar_event::SidebarEvent;
use crate::tree_state::TreeState;

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
        }
    }

    /// The geometry the preferences should store.
    pub fn panel(&self) -> &Panel {
        &self.panel
    }

    /// The preferences matching the current state.
    pub fn preferences(&self) -> Preferences {
        self.preferences.updated(&self.panel, self.mode, &self.tree)
    }

    /// Opens or closes the sidebar (Mod+B, the header button).
    pub fn toggle(&mut self, cx: &mut Context<'_, Self>) {
        self.panel.toggle();
        self.peek.slide_to(0.0, ui::Motion::of(cx));
        Self::changed(cx);
    }

    /// Chooses a mode (Mod+1, Mod+2, Mod+3).
    pub fn set_mode(&mut self, mode: Mode, cx: &mut Context<'_, Self>) {
        if !mode.is_available(self.data.focus()) {
            return;
        }
        self.mode = mode;
        Self::changed(cx);
    }

    /// Docks the sidebar to the other window edge.
    pub fn dock_to(&mut self, side: crate::dock_side::DockSide, cx: &mut Context<'_, Self>) {
        self.panel.dock_to(side);
        Self::changed(cx);
    }

    pub(crate) fn changed(cx: &mut Context<'_, Self>) {
        cx.emit(SidebarEvent::PreferencesChanged);
        cx.notify();
    }

    pub(crate) fn scroll_for(&mut self, project: &ProjectId) -> ScrollHandle {
        self.scrolls.entry(project.clone()).or_default().clone()
    }
}

impl std::fmt::Debug for Sidebar {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("Sidebar")
            .field("panel", &self.panel)
            .field("mode", &self.mode)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
#[path = "sidebar_tests.rs"]
mod tests;
