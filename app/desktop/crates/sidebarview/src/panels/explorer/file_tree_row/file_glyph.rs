use sidebar::{Expansion, FileKind, IconTheme};
use ui::{IconName, MaterialTable};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FileGlyph {
    Phosphor(IconName),
    Monochrome(String),
    Colored(&'static str),
}

impl FileGlyph {
    #[must_use]
    pub fn of(
        (theme, kind): (IconTheme, FileKind),
        name: &str,
        icons: &MaterialTable,
    ) -> Self {
        match (kind, theme) {
            (FileKind::Directory(Expansion::Expanded), _) => {
                FileGlyph::Phosphor(IconName::FolderOpen)
            },
            (FileKind::Directory(Expansion::Collapsed), _) => FileGlyph::Phosphor(IconName::Folder),
            (FileKind::File, IconTheme::Material) => {
                FileGlyph::Monochrome(icons.icon_of(name).monochrome())
            },
            (FileKind::File, IconTheme::ColorfulMaterial) => {
                FileGlyph::Colored(icons.icon_of(name).colored())
            },
            (FileKind::File, IconTheme::Classic) => FileGlyph::Phosphor(IconName::FileText),
            (FileKind::File, _) => FileGlyph::Phosphor(IconName::File),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const OPEN: FileKind = FileKind::Directory(Expansion::Expanded);
    const SHUT: FileKind = FileKind::Directory(Expansion::Collapsed);

    fn drawn_by(
        theme: IconTheme,
        icons: &MaterialTable,
    ) -> [FileGlyph; 3] {
        [FileKind::File, OPEN, SHUT].map(|kind| FileGlyph::of((theme, kind), "pricing.ts", icons))
    }

    #[test]
    fn each_theme_draws_its_file_glyph_and_every_theme_the_same_folders() {
        let icons = MaterialTable::default();
        let typescript = icons.icon_of("pricing.ts");
        let files = [
            (
                IconTheme::Material,
                FileGlyph::Monochrome(typescript.monochrome()),
            ),
            (
                IconTheme::ColorfulMaterial,
                FileGlyph::Colored(typescript.colored()),
            ),
            (IconTheme::Classic, FileGlyph::Phosphor(IconName::FileText)),
            (IconTheme::Compact, FileGlyph::Phosphor(IconName::File)),
            (IconTheme::Minimal, FileGlyph::Phosphor(IconName::File)),
            (IconTheme::None, FileGlyph::Phosphor(IconName::File)),
        ];
        let folders = [
            FileGlyph::Phosphor(IconName::FolderOpen),
            FileGlyph::Phosphor(IconName::Folder),
        ];
        for (theme, file) in files {
            let [shown, open, shut] = drawn_by(theme, &icons);
            assert_eq!((shown, [open, shut]), (file, folders.clone()), "{theme:?}");
        }
    }
}
