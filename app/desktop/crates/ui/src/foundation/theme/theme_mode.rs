//! The user's light/dark preference.

use crate::foundation::theme::appearance::Appearance;

/// The `themeMode` setting: a fixed appearance or following the OS.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
#[non_exhaustive]
pub enum ThemeMode {
    /// Always light.
    Light,
    /// Always dark.
    Dark,
    /// Follow the operating system appearance.
    #[default]
    System,
}

impl ThemeMode {
    /// The appearance this preference yields when the OS reports `system`.
    pub fn resolve(
        self,
        system: Appearance,
    ) -> Appearance {
        match self {
            Self::Light => Appearance::Light,
            Self::Dark => Appearance::Dark,
            Self::System => system,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_follows_system_only_in_system_mode() {
        const CASES: &[(ThemeMode, Appearance, Appearance)] = &[
            (ThemeMode::Light, Appearance::Dark, Appearance::Light),
            (ThemeMode::Dark, Appearance::Light, Appearance::Dark),
            (ThemeMode::System, Appearance::Dark, Appearance::Dark),
            (ThemeMode::System, Appearance::Light, Appearance::Light),
        ];
        for &(mode, system, expected) in CASES {
            assert_eq!(mode.resolve(system), expected, "{mode:?} on {system:?}");
        }
    }
}
