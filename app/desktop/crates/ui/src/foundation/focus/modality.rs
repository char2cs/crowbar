pub(in crate::foundation) mod hearing;
pub(crate) mod kept_focus;

use gpui::{App, DispatchPhase, FocusHandle, Global, MouseDownEvent, Window};

#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub struct Modality {
    keyboard: bool,
}

impl Global for Modality {}

impl Modality {
    pub fn is_keyboard(cx: &App) -> bool {
        cx.try_global::<Self>()
            .is_some_and(|modality| modality.keyboard)
    }

    pub fn rings(
        focus: &FocusHandle,
        window: &Window,
        cx: &App,
    ) -> bool {
        focus.is_focused(window) && Self::is_keyboard(cx)
    }

    pub fn install(cx: &mut App) {
        cx.intercept_keystrokes(|_, _, cx| store(Self { keyboard: true }, cx))
            .detach();
    }
}

pub(crate) fn pressed(
    _: &MouseDownEvent,
    phase: DispatchPhase,
    _: &mut Window,
    cx: &mut App,
) {
    if phase == DispatchPhase::Capture {
        store(Modality { keyboard: false }, cx);
    }
}

fn store(
    next: Modality,
    cx: &mut App,
) {
    if cx.try_global::<Modality>() == Some(&next) {
        return;
    }
    cx.set_global(next);
    cx.refresh_windows();
}

#[cfg(test)]
mod tests {
    use gpui::{
        Context, IntoElement, ParentElement, Render, Styled as _, TestAppContext,
        VisualTestContext, div, point, px,
    };

    use super::*;

    struct Root;

    impl Render for Root {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div().size_full().child(crate::hear())
        }
    }

    fn window(cx: &mut TestAppContext) -> VisualTestContext {
        cx.update(Modality::install);
        let handle = cx.add_window(|_, _| Root);
        let window = VisualTestContext::from_window(handle.into(), cx);
        window.run_until_parked();
        window
    }

    #[gpui::test]
    fn a_key_sets_the_keyboard_a_move_leaves_it_and_a_press_anywhere_ends_it(
        cx: &mut TestAppContext
    ) {
        let (mut first, mut second) = (window(cx), window(cx));
        first.simulate_keystrokes("shift");
        assert!(first.update(|_, cx| Modality::is_keyboard(cx)), "a key");
        first.simulate_mouse_move(point(px(5.0), px(5.0)), None, gpui::Modifiers::none());
        assert!(first.update(|_, cx| Modality::is_keyboard(cx)), "a move");
        second.simulate_click(point(px(5.0), px(5.0)), gpui::Modifiers::none());
        assert!(
            !first.update(|_, cx| Modality::is_keyboard(cx)),
            "a press in another window"
        );
    }
}
