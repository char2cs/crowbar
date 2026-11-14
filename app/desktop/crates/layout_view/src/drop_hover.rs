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

    pub(crate) fn preview(&self, now: Instant, motion: Motion) -> Option<Rect> {
        self.preview.as_ref().map(|tween| tween.rect(now, motion))
    }

    pub(crate) fn is_running(&self, now: Instant, motion: Motion) -> bool {
        self.preview
            .as_ref()
            .is_some_and(|tween| tween.is_running(now, motion))
    }

    pub(crate) fn command(&self, subject: &DragSubject, tab: &Tab) -> Option<model::Command> {
        self.target.as_ref()?.command(subject, tab)
    }

    pub(crate) fn over(&mut self, hit: DropHit<'_>, cx: &App) {
        let guard = self
            .overlay
            .get_or_insert_with(|| Overlays::open(cx, hit.window_bounds));
        guard.update(hit.window_bounds);
        let target = DropTarget::resolve(hit.subject, hit.point, hit.slots);
        let rect = target
            .as_ref()
            .and_then(|target| preview_rect(target, hit.slots));
        self.retarget(rect, Motion::of(cx));
        self.target = target;
    }

    pub(crate) fn clear(&mut self) {
        *self = Self::default();
    }

    fn retarget(&mut self, rect: Option<Rect>, motion: Motion) {
        let Some(rect) = rect else {
            self.preview = None;
            return;
        };
        match &mut self.preview {
            Some(tween) => tween.go_to(rect, Instant::now(), motion),
            None => self.preview = Some(RectTween::resting(rect)),
        }
    }
}

fn preview_rect(target: &DropTarget, slots: &[PaneSlot]) -> Option<Rect> {
    let slot = slots.iter().find(|slot| slot.pane == target.pane)?;
    Some(target.preview(slot.rect))
}

#[cfg(test)]
#[path = "drop_hover_tests.rs"]
mod tests;
