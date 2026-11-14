use gpui::{Bounds, Context, Entity, EventEmitter, Pixels, ScrollHandle};
use model::TabId;
use ui::Menu;

use crate::drag_session::DragSession;
use crate::incoming_drop::IncomingDrop;
use crate::new_tab::NewTab;
use crate::press::Press;
use crate::settle::Settle;
use crate::sidebar_state::SidebarState;
use crate::tab_bar_event::TabBarEvent;
use crate::tab_view::TabView;

/// The strip of view tabs: one pill per model tab, drag to reorder, drop a chat on one to merge
/// it, close with the x. It owns no tab state: it renders the [`TabView`]s it was last given and
/// answers input with [`TabBarEvent`]s.
pub struct TabBar {
    pub(crate) tabs: Vec<TabView>,
    pub(crate) active: Option<TabId>,
    pub(crate) sidebar: SidebarState,
    pub(crate) next_tab: u64,
    pub(crate) row: Bounds<Pixels>,
    pub(crate) scroll: ScrollHandle,
    pub(crate) press: Option<Press>,
    pub(crate) drag: Option<DragSession>,
    pub(crate) incoming: Option<IncomingDrop>,
    pub(crate) settle: Option<Settle>,
    pub(crate) menu: Option<Entity<Menu>>,
}

impl std::fmt::Debug for TabBar {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("TabBar")
            .field("tabs", &self.tabs.len())
            .field("active", &self.active)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<TabBarEvent> for TabBar {}

impl TabBar {
    /// An empty strip; `first_free_tab` is the first tab id the strip may hand out for tabs a
    /// drop creates, and it counts up from there.
    pub fn new(first_free_tab: TabId) -> Self {
        Self {
            tabs: Vec::new(),
            active: None,
            sidebar: SidebarState::default(),
            next_tab: first_free_tab.0,
            row: Bounds::default(),
            scroll: ScrollHandle::new(),
            press: None,
            drag: None,
            incoming: None,
            settle: None,
            menu: None,
        }
    }

    /// Shows `tabs` with `active` raised, scrolling the active tab into view when it changed.
    pub fn set_tabs(
        &mut self,
        tabs: Vec<TabView>,
        active: Option<TabId>,
        cx: &mut Context<'_, Self>,
    ) {
        let changed = self.active != active;
        if let (true, Some(index)) = (changed, active.and_then(|id| position(&tabs, id))) {
            self.scroll.scroll_to_item(index);
        }
        self.tabs = tabs;
        self.active = active;
        cx.notify();
    }

    /// Tells the strip whether the sidebar is collapsed, so it shows the re-open toggle.
    pub fn set_sidebar(&mut self, sidebar: SidebarState, cx: &mut Context<'_, Self>) {
        self.sidebar = sidebar;
        cx.notify();
    }

    pub(crate) fn fresh_tab(&mut self, title: String) -> NewTab {
        let id = TabId(self.next_tab);
        self.next_tab += 1;
        NewTab { id, title }
    }
}

pub(crate) fn position(tabs: &[TabView], id: TabId) -> Option<usize> {
    tabs.iter().position(|tab| tab.id == id)
}

#[cfg(test)]
#[path = "tab_bar_tests.rs"]
mod tests;
