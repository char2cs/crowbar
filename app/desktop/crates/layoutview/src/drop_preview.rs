use gpui::{
    App, Bounds, Corners, Hsla, InteractiveElement, IntoElement, ParentElement, Pixels, Styled,
    Window, canvas, div, px,
};
use layout::Rect;
use ui::Backdrop;

const PREVIEW_BORDER: f32 = 2.0;
const PREVIEW_FILL: f32 = 0.14;
const PREVIEW_RING: f32 = 0.25;
const BENEATH: &str = "drop-preview-beneath";

pub(crate) fn drop_preview(
    rect: Rect,
    accent: Hsla,
    window: &mut Window,
    cx: &mut App,
) -> gpui::Div {
    div()
        .absolute()
        .left(px(rect.x))
        .top(px(rect.y))
        .w(px(rect.width))
        .h(px(rect.height))
        .child(beneath(window, cx))
        .child(tinted(accent))
}

// PLATFORM: linux X11 without a compositor shows only opaque pixels; paint the window under the tint.
fn beneath(
    window: &mut Window,
    cx: &mut App,
) -> impl IntoElement {
    let backdrop = Backdrop::of(BENEATH, window, cx);
    let paint = move |bounds: Bounds<Pixels>, (), window: &mut Window, cx: &mut App| {
        backdrop.update(cx, |backdrop, _| under(backdrop, bounds, window));
    };
    canvas(|_, _, _| (), paint).absolute().size_full()
}

fn under(
    backdrop: &mut Backdrop,
    bounds: Bounds<Pixels>,
    window: &mut Window,
) {
    backdrop.paint_under(bounds, Corners::all(ui::radius::LG), window);
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
    use gpui::{TestAppContext, point, px};
    use layout::{DragSubject, Edge};

    use crate::test_support::{layout_of, open_floating};

    #[gpui::test]
    fn the_translucent_drop_preview_shows_without_a_compositor(cx: &mut TestAppContext) {
        let mut floating = open_floating(cx, &layout_of("a", &[("b", "a", Edge::Right)]));
        let (view, window) = (floating.view.clone(), &mut floating.window);
        let over = DragSubject::Chat("t1".into());
        view.update(window, |view, cx| {
            view.hover(&over, point(px(10.0), px(100.0)), cx);
        });
        window.run_until_parked();
        let preview = floating
            .top
            .debug_bounds("drop-preview")
            .unwrap_or_default();
        let shown = floating
            .top
            .update(|window, _| window.content_region().to_vec());
        assert!(
            shown
                .iter()
                .any(|region| region.contains(&preview.center())),
            "the preview is part of what the top layer shows"
        );
    }
}
