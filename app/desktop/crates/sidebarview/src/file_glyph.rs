//! The icon of a file in the file tree.

use ui::IconName;

use sidebar::{Expansion, FileKind, IconTheme};

const README_STEM: &str = "readme";
const DOCUMENT_EXTENSIONS: [&str; 5] = ["md", "mdx", "txt", "rst", "pdf"];
const COLORFUL_FOLDER: u32 = 0x00f2_c14e;

pub(crate) fn file_icon(name: &str) -> IconName {
    let (stem, extension) = name.rsplit_once('.').unwrap_or((name, ""));
    if stem.eq_ignore_ascii_case(README_STEM) {
        return IconName::Info;
    }
    if DOCUMENT_EXTENSIONS.contains(&extension.to_ascii_lowercase().as_str()) {
        return IconName::FileText;
    }
    IconName::Code
}

pub(crate) fn theme_glyph(
    theme: IconTheme,
    kind: FileKind,
    name: &str,
) -> IconName {
    match (kind, theme) {
        (FileKind::Directory(Expansion::Expanded), _) => IconName::FolderOpen,
        (FileKind::Directory(Expansion::Collapsed), _) => IconName::Folder,
        (FileKind::File, IconTheme::Material | IconTheme::ColorfulMaterial) => file_icon(name),
        (FileKind::File, IconTheme::Classic) => IconName::FileText,
        (FileKind::File, _) => IconName::File,
    }
}

pub(crate) fn folder_tint(theme: IconTheme) -> Option<u32> {
    (theme == IconTheme::ColorfulMaterial).then_some(COLORFUL_FOLDER)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_name_picks_its_icon() {
        const CASES: [(&str, IconName); 8] = [
            ("README.md", IconName::Info),
            ("readme", IconName::Info),
            ("ReadMe.txt", IconName::Info),
            ("NOTES.md", IconName::FileText),
            ("guide.PDF", IconName::FileText),
            ("main.rs", IconName::Code),
            (".gitignore", IconName::Code),
            ("README.rs.bak", IconName::Code),
        ];
        for (name, icon) in CASES {
            assert_eq!(file_icon(name), icon, "{name}");
        }
    }

    const OPEN: FileKind = FileKind::Directory(Expansion::Expanded);
    const SHUT: FileKind = FileKind::Directory(Expansion::Collapsed);

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
            assert_eq!(
                theme_glyph(theme, FileKind::File, "main.rs"),
                file,
                "{theme:?}"
            );
            assert_eq!(
                theme_glyph(theme, OPEN, "src"),
                IconName::FolderOpen,
                "{theme:?}"
            );
            assert_eq!(
                theme_glyph(theme, SHUT, "src"),
                IconName::Folder,
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
