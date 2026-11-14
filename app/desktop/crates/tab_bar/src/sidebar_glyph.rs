use gpui::prelude::FluentBuilder;
use gpui::{App, Hsla, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

const WIDTH: f32 = 18.0;
const HEIGHT: f32 = 16.0;
const RADIUS: f32 = 3.0;
const STROKE: f32 = 2.0;
const RAIL_COLUMN: f32 = 6.0;
const RAIL: f32 = 3.0;
const RAIL_HEIGHT: f32 = 1.5;
const RAIL_GAP: f32 = 2.5;

#[derive(IntoElement)]
pub(crate) struct SidebarGlyph {
    pub(crate) color: Hsla,
    pub(crate) mirrored: bool,
}

impl RenderOnce for SidebarGlyph {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let rail = || {
            div()
                .w(px(RAIL))
                .h(px(RAIL_HEIGHT))
                .rounded_full()
                .bg(self.color)
        };
        let rails = div()
            .flex()
            .flex_col()
            .items_center()
            .gap(px(RAIL_GAP))
            .pt(px(RAIL_GAP + 1.0))
            .w(px(RAIL_COLUMN))
            .child(rail())
            .child(rail());
        div()
            .flex()
            .w(px(WIDTH))
            .h(px(HEIGHT))
            .rounded(px(RADIUS))
            .border(px(STROKE))
            .border_color(self.color)
            .when(self.mirrored, Styled::flex_row_reverse)
            .child(rails)
            .child(div().h_full().w(px(STROKE)).bg(self.color))
    }
}

#[cfg(test)]
#[path = "sidebar_glyph_tests.rs"]
mod tests;
