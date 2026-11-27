#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum IconTheme {
    #[default]
    Material,
    ColorfulMaterial,
    Classic,
    Compact,
    Minimal,
    None,
}

impl IconTheme {
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
