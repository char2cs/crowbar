//! The dismiss button in the corner of a toast.

use gpui::{
    App, ElementId, InteractiveElement as _, IntoElement, ParentElement as _, RenderOnce,
    StatefulInteractiveElement as _, Styled as _, Window, div, px,
};

use crate::foundation::CloseHandler;
use crate::foundation::CubicBezier;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::Tween;
use crate::foundation::mix;
use crate::foundation::motion::hover;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::timing;

const INSET: f32 = 12.0;
const PADDING: f32 = 2.0;
const RADIUS: f32 = 4.0;
const GLYPH: f32 = 14.0;
const RESTING_OPACITY: f32 = 0.5;

#[derive(IntoElement)]
pub(crate) struct ToastClose {
    id: ElementId,
    handler: CloseHandler,
}

impl ToastClose {
    pub(crate) fn new(
        id: impl Into<ElementId>,
        handler: CloseHandler,
    ) -> Self {
        Self {
            id: id.into(),
            handler,
        }
    }
}

fn resting_opacity(hover: f32) -> f32 {
    RESTING_OPACITY + (1.0 - RESTING_OPACITY) * hover
}

impl RenderOnce for ToastClose {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let state = hover::track(&self.id, Pose::Off, window, cx);
        let tween = Tween::new(timing::DEFAULT, CubicBezier::DEFAULT);
        let level = hover::position(&state, tween, window, cx);
        let clear = palette.muted().opacity(0.0);
        let handler = self.handler;
        div()
            .id(self.id)
            .absolute()
            .top(px(INSET))
            .right(px(INSET))
            .p(px(PADDING))
            .rounded(px(RADIUS))
            .opacity(resting_opacity(level))
            .bg(mix(clear, palette.muted(), level))
            .cursor_pointer()
            .on_hover(hover::listener(&state))
            .on_click(move |_, window, cx| handler(window, cx))
            .child(Icon::new(IconName::X).size(px(GLYPH)))
    }
}

opaque_debug!(ToastClose);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_button_fades_from_half_to_full_opacity() {
        const CASES: &[(f32, f32)] = &[(0.0, 0.5), (0.5, 0.75), (1.0, 1.0)];
        for &(hover, opacity) in CASES {
            assert!((resting_opacity(hover) - opacity).abs() < 1e-6, "{hover}");
        }
    }
}
