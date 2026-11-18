//! The active theme: palette selection, mode and OS appearance.

mod appearance;
mod color_mix;
mod focus_ring;
mod palette;
mod surface;
mod theme_id;
mod theme_mode;
mod ui_text;
mod vibrancy;
mod with;

pub use appearance::Appearance;
pub(crate) use color_mix::mix;
pub use focus_ring::focus_ring;
pub use palette::Palette;
pub use surface::Surface;
pub use theme_id::ThemeId;
pub use theme_mode::ThemeMode;
pub use ui_text::UiText;
pub use vibrancy::Vibrancy;

use gpui::{App, Global};

use crate::foundation::tokens::font::{HEADING_FAMILY, MONO_FAMILY, UI_FAMILY};

/// Global holding the theme every component reads; switching it repaints all windows.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Theme {
    pub(crate) id: ThemeId,
    pub(crate) mode: ThemeMode,
    pub(crate) system: Appearance,
    pub(crate) ui_scale: f32,
    pub(crate) vibrancy: Vibrancy,
    pub(crate) ui_family: Option<&'static str>,
    pub(crate) heading_family: Option<&'static str>,
    pub(crate) mono_family: Option<&'static str>,
}

impl Global for Theme {}

impl Default for Theme {
    fn default() -> Self {
        Self {
            id: ThemeId::default(),
            mode: ThemeMode::default(),
            system: Appearance::default(),
            ui_scale: 1.0,
            vibrancy: Vibrancy::None,
            ui_family: None,
            heading_family: None,
            mono_family: None,
        }
    }
}

impl Theme {
    /// The installed theme.
    pub fn of(cx: &App) -> &Self {
        cx.global::<Self>()
    }

    /// Replaces the installed theme and repaints every window.
    pub fn install(
        self,
        cx: &mut App,
    ) {
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

    /// Family of the UI text: the chosen one, else the embedded [`UI_FAMILY`].
    pub fn ui_family(&self) -> &'static str {
        self.ui_family.unwrap_or(UI_FAMILY)
    }

    /// Family of headings: the chosen one, else the embedded [`HEADING_FAMILY`].
    pub fn heading_family(&self) -> &'static str {
        self.heading_family.unwrap_or(HEADING_FAMILY)
    }

    /// Family of code and keyboard hints: the chosen one, else the embedded [`MONO_FAMILY`].
    pub fn mono_family(&self) -> &'static str {
        self.mono_family.unwrap_or(MONO_FAMILY)
    }

    /// Palette for a region of the window.
    pub fn palette(
        &self,
        surface: Surface,
    ) -> &'static Palette {
        match (self.id, self.appearance(), surface) {
            (ThemeId::Crowbar, Appearance::Light, Surface::Chrome)
                if self.vibrancy == Vibrancy::None =>
            {
                &Palette::CROWBAR_CHROME_LIGHT_FLAT
            },
            (ThemeId::Crowbar, Appearance::Light, Surface::Window) => &Palette::CROWBAR_LIGHT,
            (ThemeId::Crowbar, Appearance::Light, Surface::Chrome) => {
                &Palette::CROWBAR_CHROME_LIGHT
            },
            (ThemeId::Crowbar, Appearance::Dark, _) => &Palette::CROWBAR_DARK,
            (ThemeId::Frosted, Appearance::Light, _) => &Palette::FROSTED_LIGHT,
            (ThemeId::Frosted, Appearance::Dark, _) => &Palette::FROSTED_DARK,
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::Hsla;

    use super::*;

    fn theme(
        id: ThemeId,
        mode: ThemeMode,
        system: Appearance,
    ) -> Theme {
        Theme::default()
            .with_id(id)
            .with_mode(mode)
            .with_system(system)
    }

    #[test]
    fn system_mode_follows_the_os() {
        let dark = theme(ThemeId::Crowbar, ThemeMode::System, Appearance::Dark);
        let light = dark.with_system(Appearance::Light);
        assert_eq!(dark.appearance(), Appearance::Dark);
        assert_eq!(light.appearance(), Appearance::Light);
        assert_ne!(
            dark.palette(Surface::Window),
            light.palette(Surface::Window)
        );
    }

    #[test]
    fn crowbar_chrome_stays_dark_in_light_mode() {
        let light = theme(ThemeId::Crowbar, ThemeMode::Light, Appearance::Light);
        let dark = theme(ThemeId::Crowbar, ThemeMode::Dark, Appearance::Light);
        assert_eq!(
            light.palette(Surface::Chrome).background(),
            dark.palette(Surface::Window).background(),
        );
        assert_ne!(
            light.palette(Surface::Chrome).background(),
            light.palette(Surface::Window).background(),
        );
    }

    #[test]
    fn chrome_washes_keep_their_alpha_only_where_the_window_is_translucent() {
        const CASES: &[(Vibrancy, bool)] =
            &[(Vibrancy::None, true), (Vibrancy::Translucent, false)];
        for &(vibrancy, opaque) in CASES {
            let themed = theme(ThemeId::Crowbar, ThemeMode::Light, Appearance::Light)
                .with_vibrancy(vibrancy);
            let chrome = themed.palette(Surface::Chrome);
            assert_eq!(
                ((chrome.chrome_bg().a - 1.0).abs() < f32::EPSILON),
                opaque,
                "{vibrancy:?} chrome_bg"
            );
            assert_eq!(
                ((chrome.pane_chrome_bg().a - 1.0).abs() < f32::EPSILON),
                opaque,
                "{vibrancy:?} pane_chrome_bg"
            );
        }
    }

    #[test]
    fn frosted_has_no_special_chrome() {
        let light = theme(ThemeId::Frosted, ThemeMode::Light, Appearance::Light);
        assert_eq!(
            light.palette(Surface::Chrome),
            light.palette(Surface::Window)
        );
    }

    #[test]
    fn spec_literals_survive() {
        let palette =
            theme(ThemeId::Crowbar, ThemeMode::Dark, Appearance::Dark).palette(Surface::Window);
        assert_eq!(
            palette.primary(),
            Hsla::from(palette::color::hex(0x98a8_6aff))
        );
        assert_eq!(
            palette.border(),
            Hsla::from(palette::color::hex(0x3333_30ff))
        );
    }
}
