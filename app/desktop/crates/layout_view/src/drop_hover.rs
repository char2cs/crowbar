use std::time::Instant;

use gpui::App;
use model::{DragSubject, PaneSlot, Rect, Tab};
use ui::{Motion, OverlayGuard, Overlays};

use crate::drop_hit::DropHit;
use crate::drop_target::DropTarget;
use crate::rect_tween::RectTween;

#[derive(Debug, Default)]
pub(crate) struct DropHover {
    target: Option<DropTarget>,
    preview: Option<RectTween>,
    overlay: Option<OverlayGuard>,
}

impl DropHover {
    pub(crate) fn is_dragging(&self) -> bool {
        self.overlay.is_some()
    }

    pub(crate) fn preview(
        &self,
        now: Instant,
        motion: Motion,
    ) -> Option<Rect> {
        self.preview.as_ref().map(|tween| tween.rect(now, motion))
    }

    pub(crate) fn is_running(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.preview
            .as_ref()
            .is_some_and(|tween| tween.is_running(now, motion))
    }

    pub(crate) fn command(
        &self,
        subject: &DragSubject,
        tab: &Tab,
    ) -> Option<model::Command> {
        self.target.as_ref()?.command(subject, tab)
    }

    pub(crate) fn over(
        &mut self,
        hit: DropHit<'_>,
        cx: &App,
    ) {
        let guard = self
            .overlay
            .get_or_insert_with(|| Overlays::open(cx, hit.window_bounds));
        guard.update(hit.window_bounds);
        let target = DropTarget::resolve(hit.subject, hit.point, hit.slots);
        let rect = target
            .as_ref()
            .and_then(|target| preview_rect(target, hit.slots));
        self.retarget(rect, (hit.now, Motion::of(cx)));
        self.target = target;
    }

    pub(crate) fn clear(&mut self) {
        *self = Self::default();
    }

    fn retarget(
        &mut self,
        rect: Option<Rect>,
        (now, motion): (Instant, Motion),
    ) {
        let Some(rect) = rect else {
            self.preview = None;
            return;
        };
        match &mut self.preview {
            Some(tween) => tween.go_to(rect, now, motion),
            None => self.preview = Some(RectTween::resting(rect)),
        }
    }
}

fn preview_rect(
    target: &DropTarget,
    slots: &[PaneSlot],
) -> Option<Rect> {
    let slot = slots.iter().find(|slot| slot.pane == target.pane)?;
    Some(target.preview(slot.rect))
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, Pixels, TestAppContext, point, px, size};
    use model::{DragSubject, Edge, PaneSlot, Rect};

    use super::*;
    use crate::test_support::{chat, layout_of, pane_id};

    const CONTENT: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 806.0,
        height: 400.0,
    };

    fn bounds() -> Bounds<Pixels> {
        Bounds::new(point(px(0.0), px(0.0)), size(px(806.0), px(400.0)))
    }

    fn hit_at<'a>(
        subject: &'a DragSubject,
        point: (f32, f32),
        slots: &'a [PaneSlot],
    ) -> DropHit<'a> {
        DropHit {
            subject,
            point,
            slots,
            window_bounds: bounds(),
            now: Instant::now(),
        }
    }

    #[gpui::test]
    fn dragging_over_a_pane_resolves_the_target_and_a_preview(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        let mut hover = DropHover::default();
        let hit = hit_at(&subject, (410.0, 200.0), &slots);
        cx.update(|cx| hover.over(hit, cx));
        assert!(hover.is_dragging(), "a drag is in progress");
        assert_eq!(
            hover.target.as_ref().map(|t| (t.pane.clone(), t.edge)),
            Some((pane_id("b"), Edge::Left)),
            "target"
        );
        assert!(
            hover.preview(Instant::now(), Motion::default()).is_some(),
            "a preview rectangle"
        );
    }

    #[gpui::test]
    fn leaving_every_pane_drops_the_target_but_the_drag_goes_on(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        let mut hover = DropHover::default();
        let on = hit_at(&subject, (410.0, 200.0), &slots);
        let off = hit_at(&subject, (403.0, 200.0), &slots);
        cx.update(|cx| {
            hover.over(on, cx);
            hover.over(off, cx);
        });
        assert!(hover.is_dragging(), "still dragging");
        assert!(hover.target.is_none(), "no target over the sash");
        assert!(
            hover.preview(Instant::now(), Motion::default()).is_none(),
            "no preview"
        );
    }

    #[gpui::test]
    fn clear_ends_the_drag(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[]);
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        let mut hover = DropHover::default();
        let hit = hit_at(&subject, (10.0, 200.0), &slots);
        cx.update(|cx| hover.over(hit, cx));
        hover.clear();
        assert!(!hover.is_dragging(), "no drag after clear");
    }
}
