use gpui::{TestAppContext, TextRun, VisualTestContext, px};

use crate::control::editor::text_area::text_area_layout::TextAreaLayout;
use crate::control::editor::text_area::text_area_layout::shaping::Shaping;

pub(crate) const LINE: f32 = 20.0;

fn run_of(text: &str) -> TextRun {
    TextRun {
        len: text.len(),
        font: gpui::font("Cal Sans UI"),
        color: gpui::black(),
        background_color: None,
        underline: None,
        strikethrough: None,
    }
}

pub(crate) fn layout_of(
    text: &'static str,
    wrap: Option<f32>,
    cx: &mut VisualTestContext,
) -> TextAreaLayout {
    let runs = [run_of(text)];
    let shaping = Shaping {
        text: text.into(),
        font_size: px(14.0),
        runs: &runs,
        wrap_width: wrap.map(px),
        line_height: px(LINE),
    };
    cx.update(|window, _| TextAreaLayout::shape(shaping, window))
}

pub(crate) fn window(cx: &mut TestAppContext) -> &mut VisualTestContext {
    crate::control::editor::test_support::area::area(cx).1
}
