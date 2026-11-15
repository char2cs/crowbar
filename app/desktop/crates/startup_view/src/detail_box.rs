use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Surface, Theme, font, radius};

const MAX_HEIGHT: f32 = 160.0;
const TEXT_SIZE: f32 = 12.0;
const LINE_HEIGHT_RATIO: f32 = 1.5;
const PADDING_X: f32 = 10.0;
const PADDING_Y: f32 = 8.0;

#[derive(Debug, IntoElement)]
pub(crate) struct DetailBox {
    id: &'static str,
    text: SharedString,
}

impl DetailBox {
    pub(crate) fn new(
        id: &'static str,
        text: impl Into<SharedString>,
    ) -> Self {
        Self {
            id,
            text: text.into(),
        }
    }
}

impl RenderOnce for DetailBox {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .id(self.id)
            .w_full()
            .max_h(px(MAX_HEIGHT))
            .overflow_y_scroll()
            .px(px(PADDING_X))
            .py(px(PADDING_Y))
            .rounded(radius::MD)
            .border_1()
            .border_color(palette.border())
            .bg(palette.muted())
            .font_family(font::MONO_FAMILY)
            .text_size(px(TEXT_SIZE))
            .line_height(px(TEXT_SIZE * LINE_HEIGHT_RATIO))
            .text_color(palette.foreground())
            .child(self.text)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn a_long_report_stops_growing_at_the_maximum_height(cx: &mut TestAppContext) {
        let report = "line\n".repeat(80);
        let bounds = measure(cx, move |_, _| {
            div()
                .w(px(400.0))
                .child(DetailBox::new("box", report.clone()))
        });
        assert_eq!(f32::from(bounds.size.height), MAX_HEIGHT);
    }

    #[gpui::test]
    fn a_short_message_hugs_its_text(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            div()
                .w(px(400.0))
                .child(DetailBox::new("box", "step failed"))
        });
        assert!(f32::from(bounds.size.height) < MAX_HEIGHT, "{bounds:?}");
    }
}
