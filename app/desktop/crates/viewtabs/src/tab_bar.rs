mod clock;
mod geometry;
mod incoming;
mod menu;
mod pointer;
mod render;
mod sidebar_glyph;
mod sidebar_state;

pub use clock::Clock;
pub use sidebar_state::SidebarState;

use std::rc::Rc;
use std::time::Instant;

use gpui::{Bounds, Context, Entity, EventEmitter, Pixels, Point, ScrollHandle};
use layout::TabId;
use ui::Menu;

use crate::gesture::DragSession;
use crate::gesture::IncomingDrop;
use crate::gesture::NewTab;
use crate::gesture::Press;
use crate::gesture::Settle;
use crate::pill::TabView;
use crate::tab_bar_event::TabBarEvent;

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
    pub(crate) menu_at: Point<Pixels>,
    pub(crate) clock: Rc<dyn Clock>,
}

impl std::fmt::Debug for TabBar {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
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
        Self::with_clock(first_free_tab, Rc::new(Instant::now))
    }

    /// Like [`TabBar::new`], but animations read the time from `clock`.
    pub fn with_clock(
        first_free_tab: TabId,
        clock: Rc<dyn Clock>,
    ) -> Self {
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
            menu_at: Point::default(),
            clock,
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
    pub fn set_sidebar(
        &mut self,
        sidebar: SidebarState,
        cx: &mut Context<'_, Self>,
    ) {
        self.sidebar = sidebar;
        cx.notify();
    }

    pub(crate) fn allocate_tab(
        &mut self,
        title: String,
    ) -> NewTab {
        let id = TabId(self.next_tab);
        self.next_tab += 1;
        NewTab { id, title }
    }
}

pub(crate) fn position(
    tabs: &[TabView],
    id: TabId,
) -> Option<usize> {
    tabs.iter().position(|tab| tab.id == id)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::TabId;

    use crate::test_support::{open, view};

    #[gpui::test]
    fn set_tabs_shows_the_tabs_and_the_active_one(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 2);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.tabs.len(), 2, "two tabs");
            assert_eq!(bar.active, Some(TabId(2)), "second is active");
        });
    }

    #[gpui::test]
    fn tabs_created_by_a_drop_get_ids_counting_up_from_the_first_free_one(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        let ids = bar.update(cx, |bar, _| {
            let first = bar.allocate_tab("One".into());
            let second = bar.allocate_tab("Two".into());
            (first.id, second.id)
        });
        assert_eq!(ids, (TabId(50), TabId(51)), "50 then 51");
    }

    #[test]
    fn position_finds_a_tab_by_id() {
        let tabs = vec![view(4, &["a"]), view(9, &["b"])];
        assert_eq!(position(&tabs, TabId(9)), Some(1), "second slot");
        assert_eq!(position(&tabs, TabId(1)), None, "unknown tab");
    }

    use super::position;
}
