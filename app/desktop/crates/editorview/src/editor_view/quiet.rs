use gpui::{AsyncApp, Context, WeakEntity};

use super::EditorView;
use crate::EditorEvent;

const QUIET_PERIOD: std::time::Duration = std::time::Duration::from_secs(1);

impl EditorView {
    pub(super) async fn wait_quiet(
        view: WeakEntity<Self>,
        cx: &mut AsyncApp,
    ) {
        cx.background_executor().timer(QUIET_PERIOD).await;
        if let Err(error) = view.update(cx, Self::edits_paused) {
            tracing::debug!(%error, "the editor closed before its edits paused");
        }
    }

    fn edits_paused(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.is_dirty(cx) {
            cx.emit(EditorEvent::EditsPaused);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::test_support::{listened, opened};

    #[gpui::test]
    fn edits_pause_once_the_quiet_period_passes_after_the_last_keystroke(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let heard = listened(&view, window);
        let paused = |heard: &std::rc::Rc<std::cell::RefCell<Vec<EditorEvent>>>| {
            heard
                .borrow()
                .iter()
                .filter(|event| **event == EditorEvent::EditsPaused)
                .count()
        };
        let almost = QUIET_PERIOD.saturating_sub(std::time::Duration::from_millis(1));
        window.simulate_input("x");
        window.executor().advance_clock(almost);
        window.simulate_input("y");
        window.executor().advance_clock(almost);
        assert_eq!(paused(&heard), 0, "still typing");
        window.executor().advance_clock(QUIET_PERIOD);
        assert_eq!(paused(&heard), 1, "quiet once");
    }
}
