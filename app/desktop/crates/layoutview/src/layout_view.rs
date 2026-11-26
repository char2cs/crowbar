pub(crate) mod drag;
mod hover_source;
mod layout_drag;
mod layout_input;
mod layout_render;
mod leaf_cell;
mod pane;
mod strip;

use std::rc::Rc;
use std::time::Instant;

use gpui::{App, Bounds, Context, EventEmitter, Pixels};
use layout::{Rect, Tab, TabId};
use protocol::PaneId;
use ui::Motion;

use crate::geometry::chrome_context::ChromeContext;
use crate::geometry::pane_frame::PaneFrame;
use crate::host::pane_source::PaneSource;
use crate::layout_event::LayoutEvent;
use crate::layout_view::drag::drop_hover::DropHover;
use crate::layout_view::drag::sash_grab::SashGrab;
use crate::layout_view::hover_source::HoverSource;
use crate::layout_view::pane::pane_cell::handle_rect;
use crate::layout_view::pane::pane_fades::PaneFades;
use crate::layout_view::pane::pane_hover::PaneHover;

/// The active tab's panes: their frames, the sashes between them, the accent ring, the pane
/// handle and the drop preview. It owns no layout state: it renders the [`Tab`] it was last
/// given and answers pointer input with [`LayoutEvent::Apply`] commands.
pub struct LayoutView {
    pub(crate) tab: Option<Tab>,
    pub(crate) chrome: ChromeContext,
    pub(crate) source: Rc<dyn PaneSource>,
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) hover: PaneHover,
    pub(crate) sash: Option<SashGrab>,
    pub(crate) drop: DropHover,
    pub(crate) rings: PaneFades,
    pub(crate) handles: PaneFades,
    pub(crate) clock: Rc<dyn Fn() -> Instant>,
}

impl std::fmt::Debug for LayoutView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("LayoutView")
            .field("tab", &self.tab.as_ref().map(Tab::id))
            .field("hovered", &self.hover.pane())
            .finish_non_exhaustive()
    }
}

impl EventEmitter<LayoutEvent> for LayoutView {}

impl LayoutView {
    /// A view of no tab yet; `source` supplies pane content, labels and new tab ids.
    pub fn new(source: Rc<dyn PaneSource>) -> Self {
        Self::with_clock(source, Rc::new(Instant::now))
    }

    pub(crate) fn with_clock(
        source: Rc<dyn PaneSource>,
        clock: Rc<dyn Fn() -> Instant>,
    ) -> Self {
        Self {
            tab: None,
            chrome: ChromeContext::default(),
            source,
            bounds: Bounds::default(),
            hover: PaneHover::default(),
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

    /// The pointer entered `pane`'s web view, which the shell learns from the page.
    pub fn page_entered(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.pointer_entered((HoverSource::Page, pane), cx);
    }

    /// Where the handle of the hovered pane hangs, relative to the view, while a split tab
    /// shows one. The shell cuts it out of the web views under it, so the handle is seen and
    /// pressed there.
    #[must_use]
    pub fn handle_hole(&self) -> Option<Rect> {
        let tab = self.tab.as_ref().filter(|tab| tab.group_count() > 1)?;
        let pane = self.hover.pane()?;
        let (rect, _) = tab
            .leaves(self.local())
            .into_iter()
            .find(|(_, group)| group.shown() == Some(pane))?;
        Some(handle_rect(&PaneFrame::new(
            rect,
            self.local(),
            self.chrome,
        )))
    }

    /// The pointer left `pane`'s web view.
    pub fn page_left(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.pointer_left((HoverSource::Page, pane), cx);
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
        let (now, motion) = ((self.clock)(), Motion::of(cx));
        let multiple = self.tab.as_ref().is_some_and(|tab| tab.group_count() > 1);
        let focused = self.tab.as_ref().and_then(|tab| tab.focused().cloned());
        let ring = focused.filter(|_| multiple);
        if let Some(pane) = &ring {
            self.rings.show(pane, now, motion);
        }
        self.rings.hide_all_but(ring.as_ref(), now, motion);
        let handle = self.hover.pane().cloned().filter(|_| multiple);
        if let Some(pane) = &handle {
            self.handles.show(pane, now, motion);
        }
        self.handles.hide_all_but(handle.as_ref(), now, motion);
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use layout::{Command, Edge, Layout};

    use super::*;
    use crate::test_support::{
        chat_row, drag_row_to, file_row, layout_of, open, open_under_row_source, pane_id,
    };

    fn side_by_side() -> Layout {
        layout_of("a", &[("b", "a", Edge::Right)])
    }

    #[gpui::test]
    fn without_a_tab_the_view_shows_the_logo_and_with_one_it_does_not(cx: &mut TestAppContext) {
        for (layout, logo) in [(Layout::default(), true), (side_by_side(), false)] {
            let harness = open(cx, &layout);
            let shown = harness
                .view
                .read_with(harness.cx, |view, _| view.shows_logo());
            assert_eq!(shown, logo);
        }
    }

    #[gpui::test]
    fn pressing_an_unfocused_pane_asks_to_focus_it_and_the_focused_one_asks_nothing(
        cx: &mut TestAppContext
    ) {
        let focus_a = vec![Command::FocusPane { pane: pane_id("a") }];
        for (pane, expected) in [("a", focus_a), ("b", vec![])] {
            let harness = open(cx, &side_by_side());
            let (view, cx) = (harness.view, harness.cx);
            view.update(cx, |view, cx| view.pane_pressed(&pane_id(pane), cx));
            assert_eq!(
                *harness.commands.borrow(),
                expected,
                "b is focused after opening"
            );
        }
    }

    #[gpui::test]
    fn dragging_a_sash_asks_the_model_to_resize_the_split(cx: &mut TestAppContext) {
        let harness = open(cx, &side_by_side());
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
        let harness = open_under_row_source(cx, &layout_of("a", &[]), chat_row());
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
            drag_row_to(cx, point(px(x), px(y)));
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
    fn a_file_row_dropped_on_a_pane_asks_to_open_it_in_that_pane_wherever_it_lands(
        cx: &mut TestAppContext
    ) {
        let harness = open_under_row_source(cx, &side_by_side(), file_row("src/a.rs"));
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let (width, height) = (f32::from(size.width), f32::from(size.height));
        for (x, y) in [(width * 0.75, height / 2.0), (width - 10.0, height - 10.0)] {
            drag_row_to(cx, point(px(x), px(y)));
        }
        let expected = LayoutEvent::OpenFile {
            path: "src/a.rs".into(),
            onto: pane_id("b"),
        };
        assert_eq!(*opened.borrow(), [expected.clone(), expected]);
    }

    #[gpui::test]
    fn detaching_asks_the_source_for_a_fresh_tab(cx: &mut TestAppContext) {
        let harness = open(cx, &side_by_side());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        view.update(cx, |view, cx| view.detach(&pane_id("a"), cx));
        let detach = Command::DetachPane {
            pane: pane_id("a"),
            new_tab: TabId(100),
            title: "Detached".into(),
        };
        assert_eq!(*commands.borrow(), [detach], "detach into a fresh tab");
    }

    fn opacities(
        view: &LayoutView,
        panes: &[(&str, f32, f32)],
    ) -> Vec<(f32, f32)> {
        let (now, motion) = ((view.clock)(), Motion::default());
        let of = |pane: &PaneId| {
            let handle = view.handles.opacity(pane, now, motion);
            (handle, view.rings.opacity(pane, now, motion))
        };
        panes.iter().map(|(pane, ..)| of(&pane_id(pane))).collect()
    }

    #[gpui::test]
    fn only_the_hovered_pane_of_a_split_shows_its_handle_and_a_lone_pane_neither(
        cx: &mut TestAppContext
    ) {
        let split: &[(&str, f32, f32)] = &[("a", 1.0, 0.0), ("b", 0.0, 1.0)];
        let lone: &[(&str, f32, f32)] = &[("a", 0.0, 0.0)];
        for (layout, expected) in [(side_by_side(), split), (layout_of("a", &[]), lone)] {
            let harness = open(cx, &layout);
            let (view, cx, clock) = (harness.view, harness.cx, harness.clock);
            view.update(cx, |view, cx| view.page_entered(&pane_id("a"), cx));
            clock.set(clock.get() + Duration::from_millis(200));
            cx.run_until_parked();
            let seen = view.read_with(cx, |view, _| opacities(view, expected));
            let want: Vec<_> = expected
                .iter()
                .map(|(_, handle, ring)| (*handle, *ring))
                .collect();
            assert_eq!(seen, want, "(handle, ring) of {expected:?}; b is focused");
        }
    }
}
