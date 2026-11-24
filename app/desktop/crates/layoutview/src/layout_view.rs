use std::rc::Rc;
use std::time::Instant;

use gpui::{App, Bounds, Context, EventEmitter, Pixels};
use layout::{Rect, Tab, TabId};
use protocol::PaneId;
use ui::Motion;

use crate::chrome_context::ChromeContext;
use crate::clock::Clock;
use crate::drop_hover::DropHover;
use crate::layout_event::LayoutEvent;
use crate::pane_fades::PaneFades;
use crate::pane_source::PaneSource;
use crate::sash_grab::SashGrab;

/// The active tab's panes: their frames, the sashes between them, the accent ring, the pane
/// handle and the drop preview. It owns no layout state: it renders the [`Tab`] it was last
/// given and answers pointer input with [`LayoutEvent::Apply`] commands.
pub struct LayoutView {
    pub(crate) tab: Option<Tab>,
    pub(crate) chrome: ChromeContext,
    pub(crate) source: Rc<dyn PaneSource>,
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) hovered: Option<PaneId>,
    pub(crate) sash: Option<SashGrab>,
    pub(crate) drop: DropHover,
    pub(crate) rings: PaneFades,
    pub(crate) handles: PaneFades,
    pub(crate) clock: Rc<dyn Clock>,
}

impl std::fmt::Debug for LayoutView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("LayoutView")
            .field("tab", &self.tab.as_ref().map(Tab::id))
            .field("hovered", &self.hovered)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<LayoutEvent> for LayoutView {}

impl LayoutView {
    /// A view of no tab yet; `source` supplies pane content, labels and new tab ids.
    pub fn new(source: Rc<dyn PaneSource>) -> Self {
        Self::with_clock(source, Rc::new(Instant::now))
    }

    /// Like [`LayoutView::new`], but animations read the time from `clock`.
    pub fn with_clock(
        source: Rc<dyn PaneSource>,
        clock: Rc<dyn Clock>,
    ) -> Self {
        Self {
            tab: None,
            chrome: ChromeContext::default(),
            source,
            bounds: Bounds::default(),
            hovered: None,
            sash: None,
            drop: DropHover::default(),
            rings: PaneFades::default(),
            handles: PaneFades::default(),
            clock,
        }
    }

    /// Shows `tab`, the tab the active space shows, or the stage when it is `None`.
    pub fn set_tab(
        &mut self,
        tab: Option<Tab>,
        cx: &mut Context<'_, Self>,
    ) {
        self.tab = tab;
        let panes = self.tab.as_ref().map_or_else(Vec::new, Tab::pane_ids);
        self.rings.retain(&panes);
        self.handles.retain(&panes);
        self.sync_fades(cx);
        cx.notify();
    }

    /// The id of the tab it shows, or `None` while it shows the stage.
    #[must_use]
    pub fn shown_tab(&self) -> Option<TabId> {
        self.tab.as_ref().map(Tab::id)
    }

    /// Tells the view what surrounds it so the outer panes get the right borders.
    pub fn set_chrome(
        &mut self,
        chrome: ChromeContext,
        cx: &mut Context<'_, Self>,
    ) {
        self.chrome = chrome;
        cx.notify();
    }

    /// The pane the pointer is over, when the shell learns it from a web view instead of from
    /// this view's own hover tracking.
    pub fn set_hovered_pane(
        &mut self,
        pane: Option<PaneId>,
        cx: &mut Context<'_, Self>,
    ) {
        self.hovered = pane;
        self.sync_fades(cx);
        cx.notify();
    }

    pub(crate) fn local(&self) -> Rect {
        Rect {
            x: 0.0,
            y: 0.0,
            width: f32::from(self.bounds.size.width),
            height: f32::from(self.bounds.size.height),
        }
    }

    pub(crate) fn measured(
        &mut self,
        bounds: Bounds<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        if self.bounds != bounds {
            self.bounds = bounds;
            cx.notify();
        }
    }

    pub(crate) fn sync_fades(
        &mut self,
        cx: &mut App,
    ) {
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        let multiple = self.tab.as_ref().is_some_and(|tab| tab.group_count() > 1);
        let focused = self.tab.as_ref().and_then(|tab| tab.focused().cloned());
        let ring = focused.filter(|_| multiple);
        if let Some(pane) = &ring {
            self.rings.show(pane, now, motion);
        }
        self.rings.hide_all_but(ring.as_ref(), now, motion);
        let handle = self.hovered.clone().filter(|_| multiple);
        if let Some(pane) = &handle {
            self.handles.show(pane, now, motion);
        }
        self.handles.hide_all_but(handle.as_ref(), now, motion);
    }
}

#[cfg(test)]
mod tests {
    use crate::layout_event::LayoutEvent;
    use crate::layout_view::LayoutView;
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use layout::{Command, DragSubject, Edge};

    use crate::test_support::{layout_of, open, open_under_row_source, pane_id};

    fn side_by_side() -> layout::Layout {
        layout_of("a", &[("b", "a", Edge::Right)])
    }

    #[gpui::test]
    fn the_drop_preview_floats_above_the_panes_until_the_drag_leaves(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, &side_by_side());
        let (view, window) = (floating.view.clone(), &mut floating.window);
        let over = DragSubject::Chat("t1".into());
        view.update(window, |view, cx| {
            view.hover(&over, point(px(10.0), px(100.0)), cx);
        });
        window.run_until_parked();
        assert_eq!(
            window.debug_bounds("drop-preview"),
            None,
            "nothing in the window"
        );
        assert!(floating.top.debug_bounds("drop-preview").is_some());
        view.update(window, |view, cx| {
            view.hover(&over, point(px(-5.0), px(-5.0)), cx);
        });
        window.update(|_, _| {});
        window.run_until_parked();
        assert_eq!(
            floating.top.debug_bounds("drop-preview"),
            None,
            "gone outside"
        );
    }

    #[gpui::test]
    fn without_a_tab_the_view_shows_the_logo_and_with_one_it_does_not(cx: &mut TestAppContext) {
        let empty = open(cx, &layout::Layout::default());
        let (view, cx) = (empty.view, empty.cx);
        view.read_with(cx, |view, _| assert!(view.shows_logo(), "no tab"));
        let populated = open(cx, &side_by_side());
        let (view, cx) = (populated.view, populated.cx);
        view.read_with(cx, |view, _| assert!(!view.shows_logo(), "a tab is open"));
    }

    #[gpui::test]
    fn the_view_measures_itself_and_lays_the_panes_out_in_its_bounds(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        cx.run_until_parked();
        view.read_with(cx, |view, _| {
            assert!(view.bounds.size.width > px(0.0), "measured width");
            let slots = view.tab.as_ref().expect("tab").layout(view.local());
            assert_eq!(slots.len(), 2, "two slots");
        });
    }

    #[gpui::test]
    fn pressing_an_unfocused_pane_asks_to_focus_it(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        view.update(cx, |view, cx| view.pane_pressed(&pane_id("a"), cx));
        cx.run_until_parked();
        assert_eq!(
            commands.borrow().as_slice(),
            &[Command::FocusPane { pane: pane_id("a") }],
            "b is focused after opening, a asks for focus"
        );
    }

    #[gpui::test]
    fn pressing_the_focused_pane_asks_for_nothing(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        view.update(cx, |view, cx| view.pane_pressed(&pane_id("b"), cx));
        assert!(commands.borrow().is_empty(), "already focused");
    }

    #[gpui::test]
    fn dragging_a_sash_asks_the_model_to_resize_the_split(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let width = view.read_with(cx, |view, _| f32::from(view.bounds.size.width));
        let sash_x = (width - 6.0) / 2.0 + 3.0;
        let at = |x: f32| point(px(x), px(200.0));
        cx.simulate_mouse_down(at(sash_x), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(sash_x + 10.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(sash_x + 60.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_up(at(sash_x + 60.0), MouseButton::Left, Modifiers::none());
        let seen = commands.borrow();
        let Some(Command::ResizeSplit { first_extent, .. }) = seen.last() else {
            panic!("expected a resize, got {seen:?}");
        };
        let expected = (width - 6.0) / 2.0 + 60.0;
        assert!(
            (first_extent - expected).abs() < 1.5,
            "first extent {first_extent} vs {expected}"
        );
    }

    #[gpui::test]
    fn dragging_a_chat_row_onto_a_pane_asks_to_open_it_at_the_zone_under_the_pointer(
        cx: &mut TestAppContext
    ) {
        let layout = layout_of("a", &[]);
        let harness = open_under_row_source(cx, &layout);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let (width, height) = (f32::from(size.width), f32::from(size.height));
        let zones = [
            ((width / 2.0, height / 2.0), Edge::Right),
            ((width / 2.0, height - 10.0), Edge::Bottom),
            ((width / 2.0, 20.0), Edge::Top),
            ((20.0, height / 2.0), Edge::Left),
        ];
        for ((x, y), _) in zones {
            let none = Modifiers::none();
            cx.simulate_mouse_down(point(px(4.0), px(4.0)), MouseButton::Left, none);
            cx.simulate_mouse_move(point(px(40.0), px(40.0)), MouseButton::Left, none);
            cx.simulate_mouse_move(point(px(x), px(y)), MouseButton::Left, none);
            cx.simulate_mouse_up(point(px(x), px(y)), MouseButton::Left, none);
        }
        let edges: Vec<_> = opened
            .borrow()
            .iter()
            .map(|event| match event {
                LayoutEvent::OpenChat {
                    chat, placement, ..
                } => (chat.clone(), placement.target.clone(), placement.edge),
                other => panic!("unexpected {other:?}"),
            })
            .collect();
        let expected: Vec<_> = zones
            .iter()
            .map(|(_, edge)| ("t1".to_owned(), pane_id("a"), *edge))
            .collect();
        assert_eq!(edges, expected, "centre, bottom, top and left");
    }

    #[gpui::test]
    fn the_handle_close_button_closes_that_pane(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        view.update(cx, |_, cx| LayoutView::close(&pane_id("a"), cx));
        assert_eq!(
            commands.borrow().as_slice(),
            &[Command::ClosePane { pane: pane_id("a") }],
            "close"
        );
    }

    #[gpui::test]
    fn detaching_asks_the_source_for_a_fresh_tab(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        view.update(cx, |view, cx| view.detach(&pane_id("a"), cx));
        assert_eq!(
            commands.borrow().as_slice(),
            &[Command::DetachPane {
                pane: pane_id("a"),
                new_tab: layout::TabId(100),
                title: "Detached".into()
            }],
            "detach into a fresh tab"
        );
    }

    #[gpui::test]
    fn only_the_hovered_pane_of_a_split_shows_its_handle(cx: &mut TestAppContext) {
        let layout = side_by_side();
        let harness = open(cx, &layout);
        let (view, cx, clock) = (harness.view, harness.cx, harness.clock);
        view.update(cx, |view, cx| view.set_hovered_pane(Some(pane_id("a")), cx));
        clock.set(clock.get() + std::time::Duration::from_millis(200));
        cx.run_until_parked();
        view.read_with(cx, |view, _| {
            let now = view.clock.now();
            let motion = ui::Motion::default();
            assert_eq!(
                view.handles.opacity(&pane_id("a"), now, motion),
                1.0,
                "a shows its handle"
            );
            assert_eq!(
                view.handles.opacity(&pane_id("b"), now, motion),
                0.0,
                "b does not"
            );
            assert_eq!(
                view.rings.opacity(&pane_id("b"), now, motion),
                1.0,
                "the focused pane has the ring"
            );
        });
    }

    #[gpui::test]
    fn a_lone_pane_has_neither_ring_nor_handle(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        view.update(cx, |view, cx| view.set_hovered_pane(Some(pane_id("a")), cx));
        view.read_with(cx, |view, _| {
            let now = view.clock.now() + std::time::Duration::from_millis(200);
            let motion = ui::Motion::default();
            assert_eq!(
                view.handles.opacity(&pane_id("a"), now, motion),
                0.0,
                "no handle"
            );
            assert_eq!(
                view.rings.opacity(&pane_id("a"), now, motion),
                0.0,
                "no ring"
            );
        });
    }
}
