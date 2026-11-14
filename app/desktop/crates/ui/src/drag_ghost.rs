//! The lifted pill that follows the pointer while a tab or chat is dragged.

use gpui::{
    Context, FontWeight, IntoElement, ParentElement, Render, SharedString, Styled, Window, div, px,
};

use crate::font::UI_FAMILY;
use crate::opaque_debug::opaque_debug;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

const HEIGHT: f32 = 32.0;
const SHADOW_ALPHA: f32 = 0.15;
const PADDING: f32 = 12.0;

/// A drag preview: the light "control surface" pill with a 1px outline and `shadow-md/15`,
/// painted from the light palette in every theme. Return it from `on_drag`.
pub struct DragGhost {
    label: SharedString,
    width: f32,
}

impl DragGhost {
    /// A ghost `width` logical pixels wide showing `label`.
    pub fn new(label: impl Into<SharedString>, width: f32) -> Self {
        Self {
            label: label.into(),
            width,
        }
    }
}

impl Render for DragGhost {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .w(px(self.width))
            .h(px(HEIGHT))
            .px(px(PADDING))
            .flex()
            .items_center()
            .overflow_hidden()
            .rounded(radius::FULL)
            .bg(palette.control_surface())
            .border_1()
            .border_color(palette.border())
            .shadow(shadow::md(SHADOW_ALPHA))
            .text_color(palette.control_foreground())
            .font_family(UI_FAMILY)
            .text_size(text_size::SM)
            .font_weight(FontWeight::MEDIUM)
            .child(self.label.clone())
    }
}

opaque_debug!(DragGhost);

#[cfg(test)]
#[path = "drag_ghost_tests.rs"]
mod tests;
