//! Which glyph set the Files panel draws, from the `iconTheme` setting.

use ui::IconName;

use crate::expansion::Expansion;
use crate::file_glyph::file_icon;
use crate::file_kind::FileKind;

const COLORFUL_FOLDER: u32 = 0x00f2_c14e;

/// The glyph set of the file tree.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum IconTheme {
    /// Glyphs chosen by file name.
    #[default]
    Material,
    /// The material glyphs with amber folders.
    ColorfulMaterial,
    /// Folders and text files.
    Classic,
    /// Folders and plain files.
    Compact,
    /// Folders and plain files.
    Minimal,
    /// Folders and plain files.
    None,
}

impl IconTheme {
    /// The theme a stored setting names; anything unknown is the default one.
    #[must_use]
    pub fn from_setting(text: &str) -> Self {
        match text {
            "colorful-material" => Self::ColorfulMaterial,
            "classic" => Self::Classic,
            "compact" => Self::Compact,
            "minimal" => Self::Minimal,
            "none" => Self::None,
            _ => Self::Material,
        }
    }

    pub(crate) fn glyph(
        self,
        kind: FileKind,
        name: &str,
    ) -> IconName {
        match (kind, self) {
            (FileKind::Directory(Expansion::Expanded), _) => IconName::FolderOpen,
            (FileKind::Directory(Expansion::Collapsed), _) => IconName::Folder,
            (FileKind::File, Self::Material | Self::ColorfulMaterial) => file_icon(name),
            (FileKind::File, Self::Classic) => IconName::FileText,
            (FileKind::File, _) => IconName::File,
        }
    }

    pub(crate) fn folder_tint(self) -> Option<u32> {
        (self == Self::ColorfulMaterial).then_some(COLORFUL_FOLDER)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const OPEN: FileKind = FileKind::Directory(Expansion::Expanded);
    const SHUT: FileKind = FileKind::Directory(Expansion::Collapsed);

    #[test]
    fn a_stored_name_picks_its_theme_and_unknown_text_the_default() {
        const CASES: [(&str, IconTheme); 7] = [
            ("material", IconTheme::Material),
            ("colorful-material", IconTheme::ColorfulMaterial),
            ("classic", IconTheme::Classic),
            ("compact", IconTheme::Compact),
            ("minimal", IconTheme::Minimal),
            ("none", IconTheme::None),
            ("neon", IconTheme::Material),
        ];
        for (text, theme) in CASES {
            assert_eq!(IconTheme::from_setting(text), theme, "{text}");
        }
    }

    #[test]
    fn each_theme_draws_its_own_file_glyph_and_every_theme_the_same_folders() {
        const FILES: [(IconTheme, IconName); 6] = [
            (IconTheme::Material, IconName::Code),
            (IconTheme::ColorfulMaterial, IconName::Code),
            (IconTheme::Classic, IconName::FileText),
            (IconTheme::Compact, IconName::File),
            (IconTheme::Minimal, IconName::File),
            (IconTheme::None, IconName::File),
        ];
        for (theme, file) in FILES {
            assert_eq!(theme.glyph(FileKind::File, "main.rs"), file, "{theme:?}");
            assert_eq!(theme.glyph(OPEN, "src"), IconName::FolderOpen, "{theme:?}");
            assert_eq!(theme.glyph(SHUT, "src"), IconName::Folder, "{theme:?}");
        }
    }

    #[test]
    fn only_the_colorful_theme_tints_folders() {
        assert!(IconTheme::ColorfulMaterial.folder_tint().is_some());
        assert_eq!(IconTheme::Material.folder_tint(), None);
        assert_eq!(IconTheme::None.folder_tint(), None);
    }
}
