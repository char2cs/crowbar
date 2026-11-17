use gpui::{Div, FontWeight, ParentElement, Styled, div, px};
use ui::{Icon, IconName, Palette, radius};

use crate::scale::{ICON_LARGE, SPACE_LG, SPACE_MD, SPACE_XS, TEXT_LARGE};
use crate::space_icon::SpaceIcon;

const TITLE_SIZE: f32 = 16.0;
const TILE: f32 = 36.0;
const ROW_PADDING: f32 = 10.0;
const TITLE: &str = "Create a Space";
const SUBTITLE: &str = "Spaces are used to organize your tabs and sessions.";

pub(crate) fn field_row(
    icon: IconName,
    label: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .flex()
        .items_center()
        .justify_between()
        .gap(SPACE_MD)
        .px(SPACE_LG)
        .py(px(ROW_PADDING))
        .rounded(radius::LG)
        .border_1()
        .border_color(palette.border())
        .bg(palette.muted())
        .text_size(TEXT_LARGE)
        .child(
            div()
                .flex()
                .items_center()
                .gap(SPACE_MD)
                .child(
                    Icon::new(icon)
                        .size(ICON_LARGE)
                        .color(palette.muted_foreground()),
                )
                .child(label),
        )
}

pub(crate) fn heading(palette: &Palette) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(SPACE_XS)
        .child(
            div()
                .text_size(px(TITLE_SIZE))
                .font_weight(FontWeight::SEMIBOLD)
                .child(TITLE),
        )
        .child(
            div()
                .text_size(TEXT_LARGE)
                .text_color(palette.muted_foreground())
                .child(SUBTITLE),
        )
}

pub(crate) fn icon_tile(
    icon: &SpaceIcon,
    palette: &Palette,
) -> Div {
    let glyph = match icon {
        SpaceIcon::Emoji(text) => div().child(text.clone()),
        SpaceIcon::Image(_) | SpaceIcon::Default => div().child(
            Icon::new(IconName::Books)
                .size(ICON_LARGE)
                .color(palette.muted_foreground()),
        ),
    };
    div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .rounded(radius::LG)
        .border_1()
        .border_dashed()
        .border_color(palette.border())
        .child(glyph)
}

#[cfg(test)]
mod tests {
    use gpui::{FlexDirection, px};
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn the_heading_stacks_and_the_icon_tile_is_a_fixed_square() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Chrome);
        let mut heading = heading(palette);
        assert_eq!(
            heading.style().flex_direction,
            Some(FlexDirection::Column),
            "title above subtitle"
        );
        let mut tile = icon_tile(&SpaceIcon::Default, palette);
        assert_eq!(tile.style().size.width, Some(px(TILE).into()), "tile width");
        let mut row = field_row(IconName::Books, "Label", palette);
        assert!(row.style().align_items.is_some(), "row centres");
    }
}
