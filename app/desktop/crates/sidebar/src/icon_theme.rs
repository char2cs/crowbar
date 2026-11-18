//! Which glyph set the Files panel draws, from the `iconTheme` setting.

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
}

#[cfg(test)]
mod tests {
    use super::*;

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
}
