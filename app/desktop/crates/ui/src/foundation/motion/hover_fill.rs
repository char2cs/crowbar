//! A background that fades in while the pointer is over an element.

use gpui::{
    App, Div, ElementId, Hsla, InteractiveElement, Stateful, StatefulInteractiveElement, Styled,
    Window,
};

use crate::foundation::motion::hover;
use crate::foundation::motion::pose::Pose;

/// A background that fades in over 150ms while the pointer is over an element.
#[derive(Debug, Clone)]
pub struct HoverFill {
    id: ElementId,
    color: Hsla,
}

impl HoverFill {
    /// A fill of `color` for the element called `id`.
    pub fn new(
        id: impl Into<ElementId>,
        color: Hsla,
    ) -> Self {
        Self {
            id: id.into(),
            color,
        }
    }

    /// Gives `base` the id and a background that fades from transparent to the colour on hover.
    pub fn apply(
        self,
        base: Div,
        (window, cx): (&mut Window, &mut App),
    ) -> Stateful<Div> {
        let state = hover::track(&self.id, Pose::Off, window, cx);
        let transparent = self.color.opacity(0.0);
        let shown = hover::color(&state, transparent..self.color, window, cx);
        base.id(self.id).bg(shown).on_hover(hover::listener(&state))
    }
}

#[cfg(test)]
mod tests {
    use std::time::Instant;

    use gpui::{
        Context, Entity, IntoElement, Render, TestAppContext, VisualTestContext, Window, div, hsla,
        point, px,
    };

    use super::*;
    use crate::foundation::motion::Motion;
    use crate::foundation::motion::cubic_bezier::CubicBezier;
    use crate::foundation::motion::toggle_state::ToggleState;
    use crate::foundation::motion::tween::Tween;
    use crate::foundation::theme::Theme;

    struct Probe {
        state: Option<Entity<ToggleState>>,
    }

    impl Render for Probe {
        fn render(
            &mut self,
            window: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            self.state = Some(hover::track(
                &ElementId::from("fill"),
                Pose::Off,
                window,
                cx,
            ));
            HoverFill::new("fill", hsla(0.0, 0.0, 0.0, 0.5))
                .apply(div().size(px(40.0)), (window, cx))
        }
    }

    fn fade_runs(
        state: &Entity<ToggleState>,
        tween: Tween,
        cx: &mut VisualTestContext,
    ) -> bool {
        state.read_with(cx, |s, _| {
            s.position(tween, Motion::default(), Instant::now()).running
        })
    }

    #[gpui::test]
    fn the_pointer_entering_starts_a_fade_on_the_element_id(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        let state = view.read_with(cx, |probe, _| probe.state.clone());
        let tween = Tween::new(
            crate::foundation::tokens::timing::DEFAULT,
            CubicBezier::DEFAULT,
        );
        let running = state.map(|state| fade_runs(&state, tween, cx));
        assert_eq!(
            running,
            Some(true),
            "a fade should run right after entering"
        );
    }

    #[gpui::test]
    fn the_pointer_leaving_the_window_fades_the_fill_out(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
        let inside = point(px(10.0), px(10.0));
        cx.simulate_mouse_move(inside, None, gpui::Modifiers::none());
        cx.simulate_event(gpui::MouseExitEvent {
            position: inside,
            pressed_button: None,
            modifiers: gpui::Modifiers::none(),
        });
        let state = view.read_with(cx, |probe, _| probe.state.clone());
        let tween = Tween::new(
            crate::foundation::tokens::timing::DEFAULT,
            CubicBezier::DEFAULT,
        );
        let later = Instant::now() + std::time::Duration::from_secs(1);
        let rest = state.map(|state| {
            state.read_with(cx, |s, _| s.position(tween, Motion::default(), later).value)
        });
        assert_eq!(
            rest,
            Some(0.0),
            "the fill should rest off once the pointer left"
        );
    }
}
