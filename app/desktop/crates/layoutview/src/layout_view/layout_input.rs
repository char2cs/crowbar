use gpui::{Context, DragMoveEvent, MouseDownEvent, Pixels, Point, Window};
use layout::{Command, Rect, Sash};
use protocol::PaneId;
use ui::SashDrag;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::layout_view::drag::sash_grab::SashGrab;
use crate::layout_view::hover_source::HoverSource;

impl LayoutView {
    pub(crate) fn grab_sash(
        &mut self,
        sash: &Sash,
        event: &MouseDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.tab.clone() else {
            return;
        };
        let local = self.local_point(event.position);
        let grab = SashGrab::begin(&tab, sash, local, self.local());
        if event.click_count < 2 {
            self.sash = grab;
            return;
        }
        self.sash = None;
        if let Some(grab) = grab {
            cx.emit(LayoutEvent::Apply(grab.reset()));
        }
    }

    pub(crate) fn sash_moved(
        &mut self,
        event: &DragMoveEvent<SashDrag>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(grab) = &self.sash else {
            return;
        };
        let command = grab.resize(self.local_point(event.event.position));
        cx.emit(LayoutEvent::Apply(command));
    }

    pub(crate) fn pointer_entered(
        &mut self,
        (source, pane): (HoverSource, &PaneId),
        cx: &mut Context<'_, Self>,
    ) {
        let before = self.handle_hole();
        self.hover.entered(source, pane);
        self.hover_changed(before, cx);
    }

    pub(crate) fn pointer_left(
        &mut self,
        (source, pane): (HoverSource, &PaneId),
        cx: &mut Context<'_, Self>,
    ) {
        let before = self.handle_hole();
        self.hover.left(source, pane);
        self.hover_changed(before, cx);
    }

    fn hover_changed(
        &mut self,
        before: Option<Rect>,
        cx: &mut Context<'_, Self>,
    ) {
        self.sync_fades(cx);
        if self.handle_hole() != before {
            cx.emit(LayoutEvent::Hovered);
        }
        cx.notify();
    }

    pub(crate) fn pane_pressed(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let focused = self.tab.as_ref().and_then(|tab| tab.focused());
        if focused == Some(pane) {
            return;
        }
        cx.emit(LayoutEvent::Apply(Command::FocusPane {
            pane: pane.clone(),
        }));
    }

    pub(crate) fn detach(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let (new_tab, title) = self.source.fresh_tab(pane);
        cx.emit(LayoutEvent::Apply(Command::DetachPane {
            pane: pane.clone(),
            new_tab,
            title,
        }));
    }
    pub(crate) fn close_group(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let members = self
            .tab
            .as_ref()
            .and_then(|tab| tab.group_of(pane))
            .map(|group| group.tabs().to_vec());
        match members {
            Some(panes) if panes.len() > 1 => {
                cx.emit(LayoutEvent::Apply(Command::ClosePanes { panes }));
            },
            _ => Self::close(pane, cx),
        }
    }

    pub(crate) fn close(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        cx.emit(LayoutEvent::Apply(Command::ClosePane {
            pane: pane.clone(),
        }));
    }

    pub(crate) fn local_point(
        &self,
        position: Point<Pixels>,
    ) -> (f32, f32) {
        let local = position - self.bounds.origin;
        (f32::from(local.x), f32::from(local.y))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};
    use layout::Edge;

    use crate::layout_event::LayoutEvent;

    use crate::layout_view::hover_source::HoverSource;
    use crate::test_support::{layout_of, open, pane_id};

    #[gpui::test]
    fn hovering_a_pane_of_a_split_marks_it_hovered_until_the_pointer_leaves(
        cx: &mut TestAppContext
    ) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        view.update(cx, |view, cx| {
            view.pointer_entered((HoverSource::Cell, &pane_id("a")), cx);
        });
        view.read_with(cx, |view, _| {
            assert_eq!(view.hover.pane(), Some(&pane_id("a")), "hovered");
        });
        view.update(cx, |view, cx| {
            view.pointer_left((HoverSource::Cell, &pane_id("a")), cx);
        });
        view.read_with(cx, |view, _| {
            assert_eq!(view.hover.pane(), None, "hover ended");
        });
    }

    #[gpui::test]
    fn leaving_a_pane_that_is_not_the_hovered_one_changes_nothing(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        view.update(cx, |view, cx| {
            view.pointer_entered((HoverSource::Cell, &pane_id("a")), cx);
        });
        view.update(cx, |view, cx| {
            view.pointer_left((HoverSource::Cell, &pane_id("b")), cx);
        });
        view.read_with(cx, |view, _| {
            assert_eq!(view.hover.pane(), Some(&pane_id("a")), "still a");
        });
    }

    #[gpui::test]
    fn the_handle_of_a_hovered_pane_is_drawn_in_the_window_over_its_hole(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let mut floating = crate::test_support::open_floating(cx, &layout);
        let (view, window) = (floating.view.clone(), &mut floating.window);
        view.update(window, |view, cx| {
            view.pointer_entered((HoverSource::Page, &pane_id("b")), cx);
        });
        let hovered = std::rc::Rc::new(std::cell::Cell::new(0));
        let counted = std::rc::Rc::clone(&hovered);
        let count = move |_, event: &LayoutEvent, _: &mut gpui::App| {
            counted.set(counted.get() + usize::from(*event == LayoutEvent::Hovered));
        };
        let _counting = window.update(|_, cx| cx.subscribe(&view, count));
        view.update(window, |view, cx| {
            view.pointer_entered((HoverSource::Cell, &pane_id("a")), cx);
            view.pointer_entered((HoverSource::Handle, &pane_id("a")), cx);
        });
        let later = floating.clock.get() + std::time::Duration::from_secs(1);
        floating.clock.set(later);
        view.update(window, |_, cx| cx.notify());
        window.run_until_parked();
        let drawn = window.debug_bounds("pane-handle").expect("in the window");
        let hole = view
            .read_with(window, |view, _| view.handle_hole())
            .expect("a hole while hovered");
        assert_eq!(
            (drawn.origin.x, drawn.origin.y, drawn.size.width),
            (px(hole.x), px(hole.y), px(hole.width)),
            "the hole is where the handle is drawn"
        );
        assert_eq!(hovered.get(), 1, "the shell is told the hole moved to a");
        let told = hovered.get();
        view.update(window, |view, cx| {
            view.pointer_left((HoverSource::Cell, &pane_id("a")), cx);
        });
        assert_eq!(hovered.get(), told, "still on the handle: nothing moved");
    }
}
