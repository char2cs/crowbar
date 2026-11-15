//! A background that fades in while the pointer is over an element.

use gpui::{
    App, Div, ElementId, Hsla, InteractiveElement, Stateful, StatefulInteractiveElement, Styled,
    Window,
};

use crate::hover;
use crate::pose::Pose;

/// Gives `base` the id `id` and a background that fades from transparent to `color` over 150ms on hover.
pub fn hover_fill(
    base: Div,
    id: impl Into<ElementId>,
    color: Hsla,
    (window, cx): (&mut Window, &mut App),
) -> Stateful<Div> {
    let id = id.into();
    let state = hover::track(&id, Pose::Off, window, cx);
    let shown = hover::color(&state, (color.opacity(0.0), color), window, cx);
    base.id(id).bg(shown).on_hover(hover::listener(&state))
}

#[cfg(test)]
mod tests {
    use std::time::Instant;

    use gpui::{
        Context, Entity, IntoElement, Render, TestAppContext, VisualTestContext, Window, div, hsla,
        point, px,
    };

    use super::*;
    use crate::cubic_bezier::CubicBezier;
    use crate::motion::Motion;
    use crate::theme::Theme;
    use crate::toggle_state::ToggleState;
    use crate::tween::Tween;

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
            hover_fill(
                div().size(px(40.0)),
                "fill",
                hsla(0.0, 0.0, 0.0, 0.5),
                (window, cx),
            )
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
        let tween = Tween::new(crate::timing::DEFAULT, CubicBezier::DEFAULT);
        let running = state.map(|state| fade_runs(&state, tween, cx));
        assert_eq!(
            running,
            Some(true),
            "a fade should run right after entering"
        );
    }
}
