use gpui::{App, Entity, Hsla, Pixels, SharedString, TextRun, TextStyle, Window};

use crate::control::editor::field::runs_for;
use crate::control::editor::text_area::layout::Layout;
use crate::control::editor::text_area::layout::Shaping;
use crate::control::editor::text_area::wrapped_element::wrapped_surface::WrappedSurface;
use crate::control::editor::text_input::PLACEHOLDER_OPACITY;

pub(crate) struct Shape<S: WrappedSurface> {
    area: Entity<S>,
    style: TextStyle,
    font_size: Pixels,
    line_height: Pixels,
    hint: Hsla,
}

impl<S: WrappedSurface> Clone for Shape<S> {
    fn clone(&self) -> Self {
        Self {
            area: self.area.clone(),
            style: self.style.clone(),
            font_size: self.font_size,
            line_height: self.line_height,
            hint: self.hint,
        }
    }
}

impl<S: WrappedSurface> Shape<S> {
    pub(crate) fn capture(
        area: Entity<S>,
        hint: Hsla,
        window: &Window,
    ) -> Self {
        let style = window.text_style();
        Self {
            area,
            font_size: style.font_size.to_pixels(window.rem_size()),
            style,
            line_height: window.line_height(),
            hint,
        }
    }

    fn shown(
        &self,
        area: &S,
    ) -> (SharedString, Hsla) {
        if area.content().is_empty() {
            let hint = self.hint.opacity(PLACEHOLDER_OPACITY);
            return (area.hint().clone(), hint);
        }
        (area.content().to_owned().into(), self.style.color)
    }

    pub(crate) fn layout(
        &self,
        wrap_width: Option<Pixels>,
        window: &Window,
        cx: &App,
    ) -> Layout {
        let area = self.area.read(cx);
        let (text, color) = self.shown(area);
        let base = TextRun {
            len: text.len(),
            font: self.style.font(),
            color,
            background_color: None,
            underline: None,
            strikethrough: None,
        };
        let marked = area.marked_range().filter(|_| !area.content().is_empty());
        let runs = runs_for(text.len(), marked, &base);
        let shaping = Shaping {
            text,
            font_size: self.font_size,
            runs: &runs,
            wrap_width,
            line_height: self.line_height,
        };
        Layout::shape(shaping, window)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, hsla, px};

    use super::*;
    use crate::control::editor::test_support::area::area;
    use crate::control::editor::text_area::TextArea;

    fn rows_of(
        entity: &Entity<TextArea>,
        wrap: f32,
        cx: &mut gpui::VisualTestContext,
    ) -> f32 {
        let hint = hsla(0.0, 0.0, 0.5, 1.0);
        cx.update(|window, cx| {
            let shape = Shape::capture(entity.clone(), hint, window);
            shape.layout(Some(px(wrap)), window, cx).height() / window.line_height()
        })
    }

    #[gpui::test]
    fn the_placeholder_is_shaped_while_empty_and_the_text_afterwards(cx: &mut TestAppContext) {
        let (entity, cx) = area(cx);
        assert_eq!(rows_of(&entity, 1000.0, cx), 1.0);
        assert!(rows_of(&entity, 40.0, cx) > 1.0, "the placeholder wraps");
        entity.update(cx, |area, cx| area.set_text("a\nb\nc", cx));
        assert_eq!(rows_of(&entity, 1000.0, cx), 3.0);
    }
}
