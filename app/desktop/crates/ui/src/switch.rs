//! The on/off switch.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, px,
};

use crate::check_handler::ToggleHandler;
use crate::color_mix::mix;
use crate::cubic_bezier::CubicBezier;
use crate::focus_ring::focus_ring;
use crate::hover;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::timing;

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
    pub fn new(id: impl Into<ElementId>, checked: bool) -> Self {
        Self { id: id.into(), checked, disabled: false, surface: Surface::default(), on_change: None }
    }

    /// Disables the switch.
    #[must_use]
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the requested value.
    #[must_use]
    pub fn on_change(mut self, handler: impl Fn(bool, &mut Window, &mut App) + 'static) -> Self {
        self.on_change = Some(Box::new(handler));
        self
    }
}

impl RenderOnce for Switch {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let state = hover::track(&self.id, "checked", self.checked, window, cx);
        state.update(cx, |s, cx| s.set(self.checked, cx));
        let slide = hover::position(&state, THUMB_SLIDE, CubicBezier::EASE, window, cx);
        let fade = hover::position(&state, timing::SLOW, CubicBezier::DEFAULT, window, cx);
        let track = mix(palette.input(), palette.primary(), fade);
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
            .bg(track)
            .when(enabled, |d| d.cursor_pointer().tab_stop(true))
            .when(!enabled, |d| d.opacity(DISABLED_OPACITY))
            .focus(move |s| s.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .child(
                div()
                    .size(px(THUMB))
                    .flex_none()
                    .ml(px(TRAVEL * slide))
                    .rounded(radius::FULL)
                    .bg(palette.background())
                    .shadow(shadow::sm(0.05)),
            )
            .when_some(self.on_change.filter(|_| enabled), move |d, handler| {
                d.on_click(move |_, window, cx| handler(!checked, window, cx))
            })
    }
}

#[cfg(test)]
#[path = "switch_tests.rs"]
mod tests;
