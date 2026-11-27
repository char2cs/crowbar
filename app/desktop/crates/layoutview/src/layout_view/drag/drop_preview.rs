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

#[cfg(test)]
mod tests {
    use gpui::{Bounds, Context, IntoElement, Render, TestAppContext, Window, point, red, size};

    use super::*;

    const RECT: Rect = Rect {
        x: 10.0,
        y: 20.0,
        width: 60.0,
        height: 30.0,
    };

    struct Shown;

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
                .relative()
                .size_full()
                .child(drop_preview(RECT, red()))
        }
    }

    #[gpui::test]
    fn the_preview_tints_exactly_the_rectangle_it_is_given(cx: &mut TestAppContext) {
        let (_, window) = cx.add_window_view(|_, _| Shown);
        window.run_until_parked();
        let expected = Bounds::new(point(px(10.0), px(20.0)), size(px(60.0), px(30.0)));
        assert_eq!(window.debug_bounds("drop-preview"), Some(expected));
    }
}
