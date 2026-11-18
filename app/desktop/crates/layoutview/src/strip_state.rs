use gpui::{App, Context, Entity, FocusHandle, Pixels, ScrollHandle, Window, px};
use protocol::PaneId;

use crate::input_mode::InputMode;

const OVERFLOW_SLACK: f32 = 1.0;
pub(crate) struct StripState {
    pub(crate) scroll: ScrollHandle,
    pub(crate) focus: FocusHandle,
    pub(crate) overflowing: bool,
    pub(crate) shown: Option<PaneId>,
    pub(crate) mode: InputMode,
}

impl StripState {
    pub(crate) fn use_mode(
        state: &Entity<Self>,
        mode: InputMode,
        cx: &mut App,
    ) {
        state.update(cx, |state, cx| state.switch_to(mode, cx));
    }

    fn switch_to(
        &mut self,
        mode: InputMode,
        cx: &mut Context<'_, Self>,
    ) {
        if self.mode == mode {
            return;
        }
        self.mode = mode;
        cx.notify();
    }

    pub(crate) fn shown_mode(
        state: &Entity<Self>,
        focus: &FocusHandle,
        (window, cx): (&Window, &App),
    ) -> InputMode {
        if focus.is_focused(window) {
            return state.read(cx).mode;
        }
        InputMode::Pointer
    }

    pub(crate) fn new(cx: &mut Context<'_, Self>) -> Self {
        Self {
            scroll: ScrollHandle::new(),
            focus: cx.focus_handle(),
            overflowing: false,
            shown: None,
            mode: InputMode::default(),
        }
    }
    pub(crate) fn follow(
        &mut self,
        pane: &PaneId,
        index: usize,
    ) {
        if self.shown.as_ref() == Some(pane) {
            return;
        }
        self.scroll.scroll_to_item(index);
        self.shown = Some(pane.clone());
    }
    pub(crate) fn measure(
        &mut self,
        spill: Pixels,
    ) -> bool {
        let overflowing = spill > px(OVERFLOW_SLACK);
        let changed = self.overflowing != overflowing;
        self.overflowing = overflowing;
        if changed {
            self.shown = None;
        }
        changed
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn the_shown_tab_is_followed_once_and_a_measure_reports_only_a_change(cx: &mut TestAppContext) {
        let state = cx.update(|cx| cx.new(StripState::new));
        let (first, second) = (PaneId::new("a").unwrap(), PaneId::new("b").unwrap());
        state.update(cx, |state, _| {
            state.follow(&first, 0);
            state.follow(&first, 3);
            assert_eq!(state.shown.as_ref(), Some(&first), "first remembered");
            state.follow(&second, 1);
            assert_eq!(state.shown.as_ref(), Some(&second), "then the second");
            assert!(state.measure(px(40.0)), "now overflowing");
            assert_eq!(
                state.shown, None,
                "the strip narrowed, so the tab is brought into view again"
            );
            assert!(!state.measure(px(40.0)), "still overflowing");
            assert!(
                state.measure(px(0.5)),
                "half a pixel is rounding, it fits again"
            );
        });
    }
}
