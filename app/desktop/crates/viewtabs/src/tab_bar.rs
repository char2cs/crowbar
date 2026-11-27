mod held;
mod incoming;
mod pointer;
mod preview;
mod render;
mod sidebar_glyph;
mod sidebar_state;

pub use sidebar_state::SidebarState;

use std::rc::Rc;
use std::time::Instant;

use gpui::{
    Bounds, Context, EventEmitter, InteractiveElement, IntoElement, MouseButton, Pixels,
    ScrollHandle, Styled, div, px,
};
use layout::TabId;

use crate::event::Event;
use crate::gesture::{DragSession, HeldGap, IncomingDrop, NewTab, Press, Settle};
use crate::pill::TabView;
use crate::span::Span;
use crate::strip_metrics::{ROW_HEIGHT, ROW_PADDING, slots};

const NO_ROW: f32 = 0.0;

/// The strip of view tabs: one pill per model tab, drag to reorder, drop a chat on one to merge
/// it, close with the x. It owns no tab state: it renders the [`TabView`]s it was last given and
/// answers input with [`Event`]s.
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
    pub(crate) gap: Option<HeldGap>,
    pub(crate) settle: Option<Settle>,
    pub(crate) clock: Rc<dyn Fn() -> Instant>,
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

impl EventEmitter<Event> for TabBar {}

impl TabBar {
    /// Whether the strip is drawn: it takes no room until a view exists.
    #[must_use]
    pub fn has_tabs(&self) -> bool {
        !self.tabs.is_empty()
    }

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
            gap: None,
            settle: None,
            clock: Rc::new(Instant::now),
        }
    }

    /// Shows `tabs` with `active` raised, scrolling the active tab into view when it changed.
    pub fn set_tabs(
        &mut self,
        tabs: Vec<TabView>,
        active: Option<TabId>,
        cx: &mut Context<'_, Self>,
    ) {
        if self.tabs == tabs && self.active == active {
            return;
        }
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

    pub(crate) fn strip_left(&self) -> Pixels {
        let leading_toggle = match self.sidebar {
            SidebarState::ClosedLeft => self.sidebar.toggle_width(),
            SidebarState::Open | SidebarState::OpenRight | SidebarState::ClosedRight => 0.0,
        };
        self.row.origin.x + px(ROW_PADDING + self.sidebar.leading_inset() + leading_toggle)
    }

    pub(crate) fn strip_width(&self) -> f32 {
        let taken = self.sidebar.leading_inset() + self.sidebar.toggle_width();
        (f32::from(self.row.size.width) - 2.0 * ROW_PADDING - taken).max(0.0)
    }

    pub(crate) fn slot_spans(&self) -> Vec<Span> {
        slots(self.tabs.len(), self.strip_width())
    }
}

pub(crate) fn position(
    tabs: &[TabView],
    id: TabId,
) -> Option<usize> {
    tabs.iter().position(|tab| tab.id == id)
}

// PLATFORM: linux hands the move to the window manager through _NET_WM_MOVERESIZE.
pub(crate) fn window_grip() -> impl IntoElement {
    div()
        .id("tab-strip-grip")
        .flex_1()
        .min_w_0()
        .h_full()
        .on_mouse_down(MouseButton::Left, ui::drag_window)
}

pub(crate) fn row_height(tabs: &[TabView]) -> f32 {
    if tabs.is_empty() {
        return NO_ROW;
    }
    ROW_HEIGHT
}

#[cfg(test)]
mod tests {
    use crate::strip_metrics::TRAFFIC_LIGHT_INSET;
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::TestAppContext;
    use layout::TabId;

    use super::*;
    use crate::test_support::{open, view};

    #[test]
    fn the_strip_takes_no_room_until_a_view_exists() {
        assert_eq!(row_height(&[]), NO_ROW, "no view");
        assert_eq!(row_height(&[view(1, &["a"])]), ROW_HEIGHT, "one view");
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

    #[gpui::test]
    fn the_same_tabs_again_do_not_redraw_the_strip(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        let notified = Rc::new(Cell::new(0));
        let count = Rc::clone(&notified);
        let _watch = cx.update(|_, cx| cx.observe(&bar, move |_, _| count.set(count.get() + 1)));
        bar.update(cx, |bar, cx| {
            bar.set_tabs(vec![view(1, &["a"])], Some(TabId(1)), cx);
        });
        assert_eq!(notified.get(), 0, "identical tabs");
        bar.update(cx, |bar, cx| {
            bar.set_tabs(vec![view(1, &["b"])], Some(TabId(1)), cx);
        });
        assert_eq!(notified.get(), 1, "a changed member");
    }

    #[gpui::test]
    fn the_strip_starts_after_the_padding_and_the_collapsed_toggle(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.read_with(cx, |bar, _| {
            let (left, width) = (f32::from(bar.row.origin.x), f32::from(bar.row.size.width));
            assert_eq!(f32::from(bar.strip_left()), left + 8.0, "8px padding");
            assert_eq!(bar.strip_width(), width - 16.0, "padding on both sides");
        });
        bar.update(cx, |bar, cx| bar.set_sidebar(SidebarState::ClosedLeft, cx));
        cx.run_until_parked();
        bar.read_with(cx, |bar, _| {
            let (left, width) = (f32::from(bar.row.origin.x), f32::from(bar.row.size.width));
            assert_eq!(
                f32::from(bar.strip_left()),
                left + 8.0 + 34.0 + TRAFFIC_LIGHT_INSET,
                "after the traffic lights, the 28px toggle and 6px gap"
            );
            assert_eq!(
                bar.strip_width(),
                width - 16.0 - 34.0 - TRAFFIC_LIGHT_INSET,
                "inset, toggle and gap come off the width"
            );
        });
    }

    #[gpui::test]
    fn a_sidebar_on_the_right_leaves_the_strip_under_the_traffic_lights(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.update(cx, |bar, cx| bar.set_sidebar(SidebarState::ClosedRight, cx));
        cx.run_until_parked();
        bar.read_with(cx, |bar, _| {
            let (left, width) = (f32::from(bar.row.origin.x), f32::from(bar.row.size.width));
            let start = left + 8.0 + TRAFFIC_LIGHT_INSET;
            assert_eq!(f32::from(bar.strip_left()), start, "past the lights");
            assert_eq!(bar.strip_width(), width - 16.0 - 34.0 - TRAFFIC_LIGHT_INSET);
        });
    }
}
