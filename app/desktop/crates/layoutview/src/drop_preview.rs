use gpui::{Hsla, InteractiveElement, ParentElement, Styled, div, px};
use layout::Rect;

const PREVIEW_BORDER: f32 = 2.0;
const PREVIEW_FILL: f32 = 0.14;
const PREVIEW_RING: f32 = 0.25;

pub(crate) fn drop_preview(
    rect: Rect,
    accent: Hsla,
) -> gpui::Div {
    div()
        .absolute()
        .left(px(rect.x))
        .top(px(rect.y))
        .w(px(rect.width))
        .h(px(rect.height))
        .child(tinted(accent))
}

fn tinted(accent: Hsla) -> gpui::Div {
    div()
        .debug_selector(|| "drop-preview".into())
        .absolute()
        .size_full()
        .rounded(ui::radius::LG)
        .border(px(PREVIEW_BORDER))
        .border_color(accent)
        .bg(accent.opacity(PREVIEW_FILL))
        .shadow(vec![gpui::BoxShadow {
            color: accent.opacity(PREVIEW_RING),
            offset: gpui::point(px(0.0), px(0.0)),
            blur_radius: px(0.0),
            spread_radius: px(1.0),
            inset: false,
        }])
}
