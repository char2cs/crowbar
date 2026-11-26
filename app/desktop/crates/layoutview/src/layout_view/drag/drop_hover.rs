use std::time::Instant;

use gpui::App;
use layout::{DragSubject, PaneSlot, Rect, Tab};
use ui::Motion;

use crate::layout_event::LayoutEvent;
use crate::layout_view::drag::drop_hit::DropHit;
use crate::layout_view::drag::drop_target::DropTarget;
use crate::layout_view::drag::rect_tween::RectTween;

#[derive(Debug, Default)]
pub(crate) struct DropHover {
    subject: Option<DragSubject>,
    target: Option<DropTarget>,
    preview: Option<RectTween>,
}

impl DropHover {
    pub(crate) fn is_dragging(&self) -> bool {
        self.subject.is_some()
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

    pub(crate) fn row_event(
        &self,
        tab: &Tab,
    ) -> Option<LayoutEvent> {
        let subject = self.subject.as_ref()?;
        matches!(subject, DragSubject::Chat(_) | DragSubject::File(_))
            .then(|| self.event(subject, tab))
            .flatten()
    }

    pub(crate) fn over_stage(
        &mut self,
        subject: &DragSubject,
    ) {
        *self = Self {
            subject: Some(subject.clone()),
            ..Self::default()
        };
    }

    pub(crate) fn stage_event(&self) -> Option<LayoutEvent> {
        match self.subject.as_ref()? {
            DragSubject::Chat(chat) => Some(LayoutEvent::StageChat { chat: chat.clone() }),
            _ => None,
        }
    }

    pub(crate) fn event(
        &self,
        subject: &DragSubject,
        tab: &Tab,
    ) -> Option<LayoutEvent> {
        let target = self.target.as_ref()?;
        match subject {
            DragSubject::Chat(chat) => Some(LayoutEvent::OpenChat {
                chat: chat.clone(),
                tab: tab.id(),
                placement: target.placement(),
            }),
            DragSubject::File(path) => Some(LayoutEvent::OpenFile {
                path: path.clone(),
                onto: target.pane.clone(),
            }),
            DragSubject::Tab(dragged) => Some(LayoutEvent::DropTab {
                tab: *dragged,
                onto: tab.id(),
                placement: target.placement(),
            }),
            _ => target.command(subject, tab).map(LayoutEvent::Apply),
        }
    }

    pub(crate) fn over(
        &mut self,
        hit: DropHit<'_>,
        cx: &App,
    ) {
        self.subject = Some(hit.subject.clone());
        let target = DropTarget::resolve(hit.subject, hit.point, hit.slots, hit.joinable);
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
    use gpui::TestAppContext;
    use layout::{DragSubject, Edge, PaneSlot, Rect};

    use super::*;
    use crate::test_support::{chat, layout_of, pane_id};

    const CONTENT: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 806.0,
        height: 400.0,
    };

    fn hit_at<'a>(
        subject: &'a DragSubject,
        x: f32,
        slots: &'a [PaneSlot],
    ) -> DropHit<'a> {
        DropHit {
            subject,
            point: (x, 200.0),
            slots,
            joinable: &[],
            now: Instant::now(),
        }
    }

    fn previewing(hover: &DropHover) -> (bool, bool) {
        let preview = hover.preview(Instant::now(), Motion::default());
        (hover.is_dragging(), preview.is_some())
    }

    #[gpui::test]
    fn a_drag_previews_the_pane_under_it_until_it_crosses_a_sash_or_ends(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.showing("").expect("tab");
        let slots = tab.layout(CONTENT);
        let chat = DragSubject::Open(chat("c"));
        let pane = DragSubject::Pane(pane_id("a"));
        let mut hover = DropHover::default();
        cx.update(|cx| hover.over(hit_at(&chat, 410.0, &slots), cx));
        assert_eq!(previewing(&hover), (true, true), "over b");
        cx.update(|cx| hover.over(hit_at(&chat, 403.0, &slots), cx));
        assert_eq!(previewing(&hover), (true, false), "over the sash");
        cx.update(|cx| hover.over(hit_at(&pane, 410.0, &slots), cx));
        assert_eq!(hover.row_event(tab), None, "a pane drag has its own drop");
        hover.clear();
        assert_eq!(previewing(&hover), (false, false), "cleared");
    }
}
