//! One redraw of the top layer, applied once the owner's frame is done.

use gpui::{AnyWindowHandle, App, FocusHandle, Window};

pub(crate) struct Redraw {
    pub(crate) focus: Option<FocusHandle>,
    pub(crate) floating: bool,
}

impl Redraw {
    pub(crate) fn on(
        self,
        window: AnyWindowHandle,
        cx: &mut App,
    ) {
        if let Err(error) = window.update(cx, |_, window, cx| self.apply(window, cx)) {
            tracing::debug!(%error, "the top layer closed before it could redraw");
        }
    }

    fn apply(
        self,
        window: &mut Window,
        cx: &mut App,
    ) {
        window.set_overlay_shown(self.floating);
        let moved_within = |focus: &FocusHandle| focus.contains_focused(window, cx);
        if let Some(focus) = self.focus.as_ref().filter(|focus| !moved_within(focus)) {
            window.focus(focus, cx);
        }
        window.refresh();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, VisualTestContext, div};

    use super::*;
    use crate::test_support::probe::open_floating;

    fn shown_after(
        top: &mut VisualTestContext,
        redraw: Redraw,
    ) -> bool {
        let window = top.update(|window, _| window.window_handle());
        top.update(|_, cx| cx.defer(move |cx| redraw.on(window, cx)));
        top.update(|window, _| window.is_overlay_shown())
    }

    #[gpui::test]
    fn a_redraw_shows_the_top_layer_only_while_something_floats(cx: &mut TestAppContext) {
        let (_, mut top) = open_floating(cx, |_, _| div());
        let floats = Redraw {
            focus: None,
            floating: true,
        };
        assert!(shown_after(&mut top, floats), "floating: shown");
        let idle = Redraw {
            focus: None,
            floating: false,
        };
        assert!(!shown_after(&mut top, idle), "nothing floats: hidden");
    }
}
