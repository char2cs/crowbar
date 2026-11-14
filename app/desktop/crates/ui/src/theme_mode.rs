//! The user's light/dark preference.

use crate::appearance::Appearance;

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
    pub fn resolve(self, system: Appearance) -> Appearance {
        match self {
            Self::Light => Appearance::Light,
            Self::Dark => Appearance::Dark,
            Self::System => system,
        }
    }
}

#[cfg(test)]
#[path = "theme_mode_tests.rs"]
mod tests;
