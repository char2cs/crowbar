//! A resolved light or dark appearance.

use gpui::WindowAppearance;

/// Resolved colour scheme of the window.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum Appearance {
    /// Light scheme.
    Light,
    /// Dark scheme; also the fallback when the OS appearance is unreadable.
    #[default]
    Dark,
}

impl From<WindowAppearance> for Appearance {
    fn from(value: WindowAppearance) -> Self {
        match value {
            WindowAppearance::Light | WindowAppearance::VibrantLight => Self::Light,
            WindowAppearance::Dark | WindowAppearance::VibrantDark => Self::Dark,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn window_appearance_maps_to_two_schemes() {
        const CASES: &[(WindowAppearance, Appearance)] = &[
            (WindowAppearance::Light, Appearance::Light),
            (WindowAppearance::VibrantLight, Appearance::Light),
            (WindowAppearance::Dark, Appearance::Dark),
            (WindowAppearance::VibrantDark, Appearance::Dark),
        ];
        for &(input, expected) in CASES {
            assert_eq!(Appearance::from(input), expected, "{input:?}");
        }
    }
}
