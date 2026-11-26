//! Whether a tooltip has taken the pointer move being dispatched, so the tooltips around it stay shut.

use gpui::{App, DispatchPhase, Global, IntoElement, MouseMoveEvent, Styled, Window, canvas};

#[derive(Debug, Default)]
pub(crate) struct TooltipClaim {
    taken: bool,
}

impl Global for TooltipClaim {}

impl TooltipClaim {
    pub(crate) fn reset_per_move() -> impl IntoElement {
        canvas(
            |_, _, _| {},
            |_, (), window, _| window.on_mouse_event(reset),
        )
        .absolute()
        .size_0()
    }

    pub(crate) fn take(cx: &mut App) -> bool {
        let claim = cx.default_global::<Self>();
        !std::mem::replace(&mut claim.taken, true)
    }
}

fn reset(
    _: &MouseMoveEvent,
    phase: DispatchPhase,
    _: &mut Window,
    cx: &mut App,
) {
    if phase == DispatchPhase::Capture {
        cx.default_global::<TooltipClaim>().taken = false;
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_first_tooltip_to_hear_a_move_takes_it(cx: &mut TestAppContext) {
        cx.update(|cx| {
            assert!(TooltipClaim::take(cx), "innermost");
            assert!(!TooltipClaim::take(cx), "the one around it");
        });
    }
}
