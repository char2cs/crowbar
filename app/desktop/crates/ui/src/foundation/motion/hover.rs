use std::ops::Range;

use gpui::{App, ElementId, Entity, Hsla, Window};

use crate::foundation::motion::Motion;
use crate::foundation::motion::adapters::clock as system_clock;
use crate::foundation::motion::cubic_bezier::CubicBezier;
use crate::foundation::motion::pose::Pose;
use crate::foundation::motion::toggle_state::ToggleState;
use crate::foundation::motion::tween::Tween;
use crate::foundation::theme::mix;

pub(crate) fn track(
    id: &ElementId,
    pose: Pose,
    window: &mut Window,
    cx: &mut App,
) -> Entity<ToggleState> {
    window.use_keyed_state((id.clone(), "toggle"), cx, |_, _| {
        ToggleState::starting(pose)
    })
}

pub(crate) fn listener(
    state: &Entity<ToggleState>
) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |hovered, _, cx| state.update(cx, |s, cx| s.set(Pose::from(*hovered), cx))
}

pub(crate) fn position(
    state: &Entity<ToggleState>,
    tween: Tween,
    window: &mut Window,
    cx: &App,
) -> f32 {
    let at = state
        .read(cx)
        .position(tween, Motion::of(cx), system_clock::now());
    if at.running {
        window.request_animation_frame();
    }
    at.value
}

pub(crate) fn color(
    state: &Entity<ToggleState>,
    shades: Range<Hsla>,
    window: &mut Window,
    cx: &App,
) -> Hsla {
    let tween = Tween::new(
        crate::foundation::tokens::timing::DEFAULT,
        CubicBezier::DEFAULT,
    );
    mix(shades.start, shades.end, position(state, tween, window, cx))
}

#[cfg(test)]
mod tests {
    use std::time::Instant;

    use gpui::{
        Context, InteractiveElement as _, IntoElement, Render, StatefulInteractiveElement as _,
        Styled as _, TestAppContext, VisualTestContext, Window, div, point, px,
    };

    use super::*;
    use crate::foundation::tokens::timing;

    struct Probe {
        state: Option<Entity<ToggleState>>,
    }

    impl Render for Probe {
        fn render(
            &mut self,
            window: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let id = ElementId::from("probe");
            let state = track(&id, Pose::Off, window, cx);
            self.state = Some(state.clone());
            div().id(id).size(px(40.0)).on_hover(listener(&state))
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
    fn pointer_enter_starts_a_fade(cx: &mut TestAppContext) {
        let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        let state = view.read_with(cx, |probe, _| probe.state.clone());
        let tween = Tween::new(timing::DEFAULT, CubicBezier::DEFAULT);
        let running = state.map(|state| fade_runs(&state, tween, cx));
        assert_eq!(
            running,
            Some(true),
            "a fade should be running right after entering"
        );
    }
}
