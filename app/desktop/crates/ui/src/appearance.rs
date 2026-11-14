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
#[path = "appearance_tests.rs"]
mod tests;
