//! Keyboard shortcut hint.

use gpui::{App, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px};

use crate::font::UI_FAMILY;
use crate::radius;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::text_size::UiText;

const MIN_HEIGHT: f32 = 16.0;

/// The `kbd` pill showing a shortcut such as `Ctrl+B`.
#[derive(Debug, Clone, IntoElement)]
pub struct Kbd {
    keys: SharedString,
}

impl Kbd {
    /// A hint for already formatted `keys`.
    pub fn new(keys: impl Into<SharedString>) -> Self {
        Self { keys: keys.into() }
    }
}

impl RenderOnce for Kbd {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .min_h(px(MIN_HEIGHT))
            .px(px(6.0))
            .rounded(radius::MD)
            .border_1()
            .border_color(palette.border())
            .bg(palette.card())
            .text_color(palette.muted_foreground())
            .text_size(UiText::Sm.size(&theme))
            .font_family(UI_FAMILY)
            .line_height(px(MIN_HEIGHT - 2.0))
            .child(self.keys)
    }
}

#[cfg(test)]
#[path = "kbd_tests.rs"]
mod tests;
