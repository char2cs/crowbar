//! The active theme: palette selection, mode and OS appearance.

use gpui::{App, Global};

use crate::appearance::Appearance;
use crate::palette::Palette;
use crate::surface::Surface;
use crate::theme_id::ThemeId;
use crate::theme_mode::ThemeMode;

/// Global holding the theme every component reads; switching it repaints all windows.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Theme {
    id: ThemeId,
    mode: ThemeMode,
    system: Appearance,
    ui_scale: f32,
}

impl Global for Theme {}

impl Default for Theme {
    fn default() -> Self {
        Self {
            id: ThemeId::default(),
            mode: ThemeMode::default(),
            system: Appearance::default(),
            ui_scale: 1.0,
        }
    }
}

impl Theme {
    /// The installed theme.
    pub fn of(cx: &App) -> &Self {
        cx.global::<Self>()
    }

    /// Replaces the installed theme and repaints every window.
    pub fn install(self, cx: &mut App) {
        cx.set_global(self);
        cx.refresh_windows();
    }

    /// Theme id.
    pub fn id(&self) -> ThemeId {
        self.id
    }

    /// Light/dark preference.
    pub fn mode(&self) -> ThemeMode {
        self.mode
    }

    /// The appearance after applying the mode to the OS appearance.
    pub fn appearance(&self) -> Appearance {
        self.mode.resolve(self.system)
    }

    /// `--app-ui-scale`: UI font size divided by 15.
    pub fn ui_scale(&self) -> f32 {
        self.ui_scale
    }

    /// The same theme with another id.
    #[must_use]
    pub fn with_id(self, id: ThemeId) -> Self {
        Self { id, ..self }
    }

    /// The same theme with another mode.
    #[must_use]
    pub fn with_mode(self, mode: ThemeMode) -> Self {
        Self { mode, ..self }
    }

    /// The same theme after the OS reported `system`.
    #[must_use]
    pub fn with_system(self, system: Appearance) -> Self {
        Self { system, ..self }
    }

    /// The same theme for a UI font size in points (`uiFontSize` setting, 10 to 24).
    #[must_use]
    pub fn with_ui_font_size(self, size: f32) -> Self {
        Self {
            ui_scale: size.clamp(10.0, 24.0) / DEFAULT_UI_FONT_SIZE,
            ..self
        }
    }

    /// Palette for a region of the window.
    pub fn palette(&self, surface: Surface) -> &'static Palette {
        match (self.id, self.appearance(), surface) {
            (ThemeId::Crowbar, Appearance::Light, Surface::Window) => &Palette::CROWBAR_LIGHT,
            (ThemeId::Crowbar, Appearance::Light, Surface::Chrome) => &Palette::CROWBAR_CHROME_LIGHT,
            (ThemeId::Crowbar, Appearance::Dark, _) => &Palette::CROWBAR_DARK,
            (ThemeId::Frosted, Appearance::Light, _) => &Palette::FROSTED_LIGHT,
            (ThemeId::Frosted, Appearance::Dark, _) => &Palette::FROSTED_DARK,
        }
    }
}

const DEFAULT_UI_FONT_SIZE: f32 = 15.0;

#[cfg(test)]
#[path = "theme_tests.rs"]
mod tests;
