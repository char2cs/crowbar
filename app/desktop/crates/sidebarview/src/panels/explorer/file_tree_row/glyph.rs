//! The icon of a file in the file tree.

use gpui::{AnyElement, App, Div, Hsla, IntoElement, ParentElement, Styled, div};
use ui::{Icon, IconName, IconWeight, MaterialTable, Palette, ProviderGlyph};

use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::file_tree_row::TreeLook;
use crate::sidebar::{ICON_LARGE, ICON_MEDIUM};

use sidebar::{FileKind, IconTheme};

use crate::panels::explorer::file_tree_row::file_glyph::FileGlyph;

const COLORFUL_FOLDER: u32 = 0x00f2_c14e;

pub(crate) fn folder_tint(theme: IconTheme) -> Option<u32> {
    (theme == IconTheme::ColorfulMaterial).then_some(COLORFUL_FOLDER)
}

pub(crate) fn glyph(
    entry: &FileTreeEntry,
    (look, palette): (&TreeLook, &Palette),
    cx: &mut App,
) -> Div {
    let ink = palette.muted_foreground();
    let tint = folder_tint(look.icons).filter(|_| matches!(entry.kind, FileKind::Directory(_)));
    let icons = cx.default_global::<MaterialTable>();
    let drawn = match FileGlyph::of((look.icons, entry.kind), &entry.name, icons) {
        FileGlyph::Phosphor(name) => phosphor(name, (ink, tint)),
        FileGlyph::Monochrome(svg) => ProviderGlyph::new(svg, ink)
            .size(ICON_MEDIUM)
            .into_any_element(),
        FileGlyph::Colored(svg) => ProviderGlyph::new(svg, ink)
            .size(ICON_MEDIUM)
            .into_any_element(),
    };
    div()
        .flex()
        .flex_none()
        .size(ICON_LARGE)
        .items_center()
        .justify_center()
        .child(drawn)
}

fn phosphor(
    name: IconName,
    (ink, tint): (Hsla, Option<u32>),
) -> AnyElement {
    let icon = Icon::new(name).size(ICON_MEDIUM).color(ink);
    let Some(rgb) = tint else {
        return icon.into_any_element();
    };
    icon.weight(IconWeight::Fill)
        .color(gpui::rgb(rgb).into())
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_colorful_theme_tints_folders() {
        assert!(folder_tint(IconTheme::ColorfulMaterial).is_some());
        assert_eq!(folder_tint(IconTheme::Material), None);
        assert_eq!(folder_tint(IconTheme::None), None);
    }
}
