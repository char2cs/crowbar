//! The name card that follows the pointer while a Files row is dragged.

use std::rc::Rc;

use gpui::{
    AnyElement, App, AppContext as _, BoxShadow, Entity, InteractiveElement, IntoElement,
    ParentElement, Pixels, Point, SharedString, Styled, div, hsla, point, px,
};
use ui::{Surface, Theme, radius};

use crate::panels::chats::{GhostBuild, RowGhost};
use crate::sidebar::{SPACE_LG, SPACE_SM, TEXT_SMALL};

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts
const BESIDE_POINTER: Point<Pixels> = point(px(10.0), px(-10.0));
const OPACITY: f32 = 0.95;
const SHADOW_DROP: Pixels = px(4.0);
const SHADOW_BLUR: Pixels = px(12.0);
const SHADOW_ALPHA: f32 = 0.15;

pub(crate) fn file_ghost(
    name: SharedString,
    grabbed: Point<Pixels>,
    cx: &mut App,
) -> Entity<RowGhost> {
    let build: GhostBuild = Rc::new(move |cx| card(name.clone(), grabbed, cx));
    cx.new(|_| RowGhost::new(build, radius::SM))
}

fn card(
    name: SharedString,
    grabbed: Point<Pixels>,
    cx: &App,
) -> AnyElement {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let at = grabbed + BESIDE_POINTER;
    let shadow = BoxShadow {
        color: hsla(0.0, 0.0, 0.0, SHADOW_ALPHA),
        offset: point(px(0.0), SHADOW_DROP),
        blur_radius: SHADOW_BLUR,
        spread_radius: px(0.0),
        inset: false,
    };
    div()
        .relative()
        .left(at.x)
        .top(at.y)
        .py(SPACE_SM)
        .px(SPACE_LG)
        .whitespace_nowrap()
        .opacity(OPACITY)
        .bg(palette.control_surface())
        .border_1()
        .border_color(palette.border())
        .rounded(radius::SM)
        .shadow(vec![shadow])
        .font_family(theme.ui_family())
        .text_size(TEXT_SMALL)
        .text_color(palette.control_foreground())
        .debug_selector(|| "file-ghost".into())
        .child(name)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::mount;

    #[gpui::test]
    fn the_card_sits_ten_pixels_right_of_and_above_the_pointer(cx: &mut TestAppContext) {
        let grabbed = point(px(40.0), px(13.0));
        let cx = mount(cx, move |_, cx| file_ghost("lib.rs".into(), grabbed, cx));
        let bounds = cx.debug_bounds("file-ghost").unwrap_or_default();
        assert_eq!(bounds.origin, point(px(50.0), px(3.0)));
        let padded = px(2.0 * 12.0 + 2.0);
        assert!(bounds.size.width > padded, "the name is drawn: {bounds:?}");
    }
}
