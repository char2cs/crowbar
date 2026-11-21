//! The icon of a file in the file tree.

use gpui::{AnyElement, App, Div, Hsla, IntoElement, ParentElement, Styled, div};
use ui::{Icon, IconName, IconWeight, MaterialIcon, Palette, ProviderGlyph};

use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::file_tree_row::TreeLook;
use crate::sidebar::{ICON_LARGE, ICON_MEDIUM};

use sidebar::{Expansion, FileKind, IconTheme};

const COLORFUL_FOLDER: u32 = 0x00f2_c14e;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Glyph {
    Phosphor(IconName),
    Monochrome(String),
    Colored(&'static str),
}

pub(crate) fn theme_glyph(
    theme: IconTheme,
    kind: FileKind,
    name: &str,
    cx: &mut App,
) -> Glyph {
    match (kind, theme) {
        (FileKind::Directory(Expansion::Expanded), _) => Glyph::Phosphor(IconName::FolderOpen),
        (FileKind::Directory(Expansion::Collapsed), _) => Glyph::Phosphor(IconName::Folder),
        (FileKind::File, IconTheme::Material) => {
            Glyph::Monochrome(MaterialIcon::of(name, cx).monochrome())
        },
        (FileKind::File, IconTheme::ColorfulMaterial) => {
            Glyph::Colored(MaterialIcon::of(name, cx).colored())
        },
        (FileKind::File, IconTheme::Classic) => Glyph::Phosphor(IconName::FileText),
        (FileKind::File, _) => Glyph::Phosphor(IconName::File),
    }
}

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
    let drawn = match theme_glyph(look.icons, entry.kind, &entry.name, cx) {
        Glyph::Phosphor(name) => phosphor(name, (ink, tint)),
        Glyph::Monochrome(svg) => ProviderGlyph::new(svg, ink)
            .size(ICON_MEDIUM)
            .into_any_element(),
        Glyph::Colored(svg) => ProviderGlyph::new(svg, ink)
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
    use gpui::TestAppContext;

    use super::*;

    const OPEN: FileKind = FileKind::Directory(Expansion::Expanded);
    const SHUT: FileKind = FileKind::Directory(Expansion::Collapsed);

    fn drawn_by(
        theme: IconTheme,
        cx: &mut App,
    ) -> [Glyph; 3] {
        [FileKind::File, OPEN, SHUT].map(|kind| theme_glyph(theme, kind, "pricing.ts", cx))
    }

    #[gpui::test]
    fn each_theme_draws_its_file_glyph_and_every_theme_the_same_folders(cx: &mut TestAppContext) {
        let (mono, colored) = cx.update(|cx| {
            let typescript = MaterialIcon::of("pricing.ts", cx);
            (typescript.monochrome(), typescript.colored())
        });
        let files = [
            (IconTheme::Material, Glyph::Monochrome(mono)),
            (IconTheme::ColorfulMaterial, Glyph::Colored(colored)),
            (IconTheme::Classic, Glyph::Phosphor(IconName::FileText)),
            (IconTheme::Compact, Glyph::Phosphor(IconName::File)),
            (IconTheme::Minimal, Glyph::Phosphor(IconName::File)),
            (IconTheme::None, Glyph::Phosphor(IconName::File)),
        ];
        for (theme, file) in files {
            let drawn = cx.update(|cx| drawn_by(theme, cx));
            let folders = [
                Glyph::Phosphor(IconName::FolderOpen),
                Glyph::Phosphor(IconName::Folder),
            ];
            assert_eq!(
                drawn,
                [file, folders[0].clone(), folders[1].clone()],
                "{theme:?}"
            );
        }
    }

    #[test]
    fn only_the_colorful_theme_tints_folders() {
        assert!(folder_tint(IconTheme::ColorfulMaterial).is_some());
        assert_eq!(folder_tint(IconTheme::Material), None);
        assert_eq!(folder_tint(IconTheme::None), None);
    }
}
