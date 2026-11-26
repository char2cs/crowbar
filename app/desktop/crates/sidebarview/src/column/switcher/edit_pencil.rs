//! The pencil that fades over an editable mark while the pointer is on the mark, as develop's does.

use gpui::{
    AnyElement, InteractiveElement as _, IntoElement, ParentElement, Pixels, Styled, black, div,
    hsla, px, white,
};
use ui::{Icon, IconName, radius};

const SHADE_ALPHA: f32 = 0.45;
const PENCIL_SIZE: f32 = 10.0;
pub(crate) const GROUP: &str = "space-tile";

pub(crate) fn edit_pencil(size: Pixels) -> AnyElement {
    let overlay = div()
        .absolute()
        .top_0()
        .left_0()
        .flex()
        .items_center()
        .justify_center()
        .size(size)
        .rounded(radius::MD)
        .bg(hsla(black().h, black().s, black().l, SHADE_ALPHA))
        .opacity(0.0)
        .group_hover(GROUP, |style| style.opacity(1.0))
        .child(
            Icon::new(IconName::PencilSimple)
                .size(px(PENCIL_SIZE))
                .color(white()),
        );
    overlay.into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div, px};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_overlay_floats_over_the_mark_without_taking_room(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            div()
                .relative()
                .size(px(20.0))
                .child(div().size(px(20.0)))
                .child(edit_pencil(px(20.0)))
        });
        assert_eq!(bounds.size.height, px(20.0));
    }
}
