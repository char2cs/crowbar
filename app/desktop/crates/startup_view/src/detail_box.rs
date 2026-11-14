use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Surface, Theme, font, radius};

const MAX_HEIGHT: f32 = 160.0;
const TEXT_SIZE: f32 = 12.0;

#[derive(Debug, IntoElement)]
pub(crate) struct DetailBox {
    id: &'static str,
    text: SharedString,
}

impl DetailBox {
    pub(crate) fn new(id: &'static str, text: impl Into<SharedString>) -> Self {
        Self {
            id,
            text: text.into(),
        }
    }
}

impl RenderOnce for DetailBox {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .id(self.id)
            .w_full()
            .max_h(px(MAX_HEIGHT))
            .overflow_y_scroll()
            .px(px(10.0))
            .py(px(8.0))
            .rounded(radius::MD)
            .border_1()
            .border_color(palette.border())
            .bg(palette.muted())
            .font_family(font::MONO_FAMILY)
            .text_size(px(TEXT_SIZE))
            .line_height(px(TEXT_SIZE * 1.5))
            .text_color(palette.foreground())
            .child(self.text)
    }
}

#[cfg(test)]
#[path = "detail_box_tests.rs"]
mod tests;
