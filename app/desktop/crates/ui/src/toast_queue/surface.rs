//! What a toast column shows and the swipe that dismisses a toast.

use gpui::{Context, Pixels, Point};

use super::ToastQueue;
use crate::toast_id::ToastId;
use crate::toast_placement::ToastPlacement;
use crate::toast_swipe::ToastSwipe;

impl ToastQueue {
    pub(crate) fn begin_swipe(
        &mut self,
        toast: ToastId,
        origin: Point<Pixels>,
    ) {
        self.swipe = Some(ToastSwipe { toast, origin });
    }

    pub(crate) fn drag_swipe(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(swipe) = self.swipe else {
            return;
        };
        if !swipe.dismisses(position, self.placement.side()) {
            return;
        }
        self.swipe = None;
        self.dismiss(swipe.toast, cx);
    }

    /// Moves the column, for example when the sidebar collapses.
    pub fn set_placement(
        &mut self,
        placement: ToastPlacement,
        cx: &mut Context<'_, Self>,
    ) {
        self.placement = placement;
        cx.notify();
    }

    /// Where the column sits now.
    pub fn placement(&self) -> ToastPlacement {
        self.placement
    }

    /// How many toasts the queue holds, leaving ones included.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether the queue holds no toast.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::toast_content::ToastContent;
    use crate::toast_kind::ToastKind;
    use crate::toast_lifetime::ToastLifetime;

    #[gpui::test]
    fn a_swipe_past_the_threshold_dismisses_the_toast(cx: &mut TestAppContext) {
        let queue = cx.new(|_| ToastQueue::new(ToastPlacement::default()));
        let id = queue.update(cx, |queue, cx| {
            queue.push(
                ToastContent::new(ToastKind::Info, "Saved"),
                ToastLifetime::Pinned,
                cx,
            )
        });
        queue.update(cx, |queue, cx| {
            queue.begin_swipe(id, gpui::point(gpui::px(200.0), gpui::px(10.0)));
            queue.drag_swipe(gpui::point(gpui::px(180.0), gpui::px(10.0)), cx);
        });
        let leaving = queue.read_with(cx, |queue, _| {
            queue.entries.first().is_some_and(|e| e.leaving)
        });
        assert!(!leaving, "20px is below the threshold");
        queue.update(cx, |queue, cx| {
            queue.drag_swipe(gpui::point(gpui::px(150.0), gpui::px(10.0)), cx);
        });
        let leaving = queue.read_with(cx, |queue, _| {
            queue.entries.first().is_some_and(|e| e.leaving)
        });
        assert!(leaving);
    }
}
