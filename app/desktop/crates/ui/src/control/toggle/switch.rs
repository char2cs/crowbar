//! The on/off switch.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::control::toggle::handler::ToggleHandler;
use crate::foundation::CubicBezier;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::Tween;
use crate::foundation::focus_ring;
use crate::foundation::mix;
use crate::foundation::motion::hover;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::timing;

const TRACK_WIDTH: f32 = 30.0;
const TRACK_HEIGHT: f32 = 18.0;
const THUMB: f32 = 16.0;
const TRAVEL: f32 = 12.0;
const DISABLED_OPACITY: f32 = 0.64;
const THUMB_SLIDE: std::time::Duration = std::time::Duration::from_millis(150);

/// A 30x18 switch; the thumb slides 12px and the track fades to the primary colour.
#[derive(IntoElement)]
pub struct Switch {
    id: ElementId,
    checked: bool,
    disabled: bool,
    surface: Surface,
    on_change: Option<ToggleHandler>,
}

impl Switch {
    /// A switch showing `checked`; the owner stores the value and passes it back.
    pub fn new(
        id: impl Into<ElementId>,
        checked: impl Into<Pose>,
    ) -> Self {
        Self {
            id: id.into(),
            checked: bool::from(checked.into()),
            disabled: false,
            surface: Surface::default(),
            on_change: None,
        }
    }

    /// Disables the switch.
    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the requested value.
    #[must_use]
    pub fn on_change(
        mut self,
        handler: impl Fn(Pose, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_change = Some(Box::new(handler));
        self
    }
}

fn thumb(
    offset: f32,
    palette: &Palette,
) -> gpui::Div {
    div()
        .size(px(THUMB))
        .flex_none()
        .ml(px(TRAVEL * offset))
        .rounded(radius::FULL)
        .bg(palette.background())
        .shadow(shadow::sm(opacity::PERCENT_5))
}

impl RenderOnce for Switch {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let pose = Pose::from(self.checked);
        let state = hover::track(&self.id, pose, window, cx);
        state.update(cx, |s, cx| s.set(pose, cx));
        let slide = hover::position(
            &state,
            Tween::new(THUMB_SLIDE, CubicBezier::EASE),
            window,
            cx,
        );
        let fade = hover::position(
            &state,
            Tween::new(timing::SLOW, CubicBezier::DEFAULT),
            window,
            cx,
        );
        let (checked, enabled) = (self.checked, !self.disabled);
        div()
            .id(self.id)
            .flex()
            .flex_none()
            .items_center()
            .w(px(TRACK_WIDTH))
            .h(px(TRACK_HEIGHT))
            .p(px(1.0))
            .rounded(radius::FULL)
            .bg(mix(palette.input(), palette.primary(), fade))
            .when(enabled, |d| d.cursor_pointer().tab_stop(true))
            .when(!enabled, |d| d.opacity(DISABLED_OPACITY))
            .focus(move |s| s.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .child(thumb(slide, palette))
            .when_some(self.on_change.filter(|_| enabled), move |d, handler| {
                d.on_click(move |_, window, cx| handler(Pose::from(!checked), window, cx))
            })
    }
}

opaque_debug!(Switch);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::test_support::probe::{measure, open};

    #[gpui::test]
    fn track_is_thirty_by_eighteen(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| Switch::new("s", true));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (30.0, 18.0)
        );
    }

    fn requested(
        cx: &mut TestAppContext,
        checked: Pose,
    ) -> Option<bool> {
        let seen = Rc::new(Cell::new(None));
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Switch::new("s", checked)
                .on_change(move |value, _, _| sink.set(Some(bool::from(value))))
        });
        cx.simulate_click(point(px(10.0), px(9.0)), gpui::Modifiers::none());
        seen.get()
    }

    #[gpui::test]
    fn click_requests_the_opposite_value(cx: &mut TestAppContext) {
        const CASES: &[(Pose, Option<bool>)] = &[(Pose::On, Some(false)), (Pose::Off, Some(true))];
        for &(checked, expected) in CASES {
            assert_eq!(requested(cx, checked), expected, "checked {checked:?}");
        }
    }

    #[gpui::test]
    fn disabled_switch_ignores_clicks(cx: &mut TestAppContext) {
        let seen = Rc::new(Cell::new(false));
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Switch::new("s", true)
                .disabled()
                .on_change(move |_, _, _| sink.set(true))
        });
        cx.simulate_click(point(px(10.0), px(9.0)), gpui::Modifiers::none());
        assert!(!seen.get());
    }
}
