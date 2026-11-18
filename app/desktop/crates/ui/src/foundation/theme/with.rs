use crate::foundation::theme::Theme;
use crate::foundation::theme::appearance::Appearance;
use crate::foundation::theme::theme_id::ThemeId;
use crate::foundation::theme::theme_mode::ThemeMode;
use crate::foundation::theme::vibrancy::Vibrancy;

const DEFAULT_UI_FONT_SIZE: f32 = 15.0;
const MIN_UI_FONT_SIZE: f32 = 10.0;
const MAX_UI_FONT_SIZE: f32 = 24.0;

impl Theme {
    /// The same theme with another id.
    #[must_use]
    pub fn with_id(
        self,
        id: ThemeId,
    ) -> Self {
        Self { id, ..self }
    }

    /// The same theme with another mode.
    #[must_use]
    pub fn with_mode(
        self,
        mode: ThemeMode,
    ) -> Self {
        Self { mode, ..self }
    }

    /// The same theme after the OS reported `system`.
    #[must_use]
    pub fn with_system(
        self,
        system: Appearance,
    ) -> Self {
        Self { system, ..self }
    }

    /// The same theme for a window with or without a blurred backdrop.
    #[must_use]
    pub fn with_vibrancy(
        self,
        vibrancy: Vibrancy,
    ) -> Self {
        Self { vibrancy, ..self }
    }

    /// The same theme for a UI font size in points (`uiFontSize` setting, 10 to 24).
    #[must_use]
    pub fn with_ui_font_size(
        self,
        size: f32,
    ) -> Self {
        Self {
            ui_scale: size.clamp(MIN_UI_FONT_SIZE, MAX_UI_FONT_SIZE) / DEFAULT_UI_FONT_SIZE,
            ..self
        }
    }

    /// The same theme with `family` for the UI text (`uiFontFamily` setting).
    #[must_use]
    pub fn with_ui_family(
        self,
        family: &'static str,
    ) -> Self {
        Self {
            ui_family: Some(family),
            ..self
        }
    }

    /// The same theme with `family` for headings (`headingFontFamily` setting).
    #[must_use]
    pub fn with_heading_family(
        self,
        family: &'static str,
    ) -> Self {
        Self {
            heading_family: Some(family),
            ..self
        }
    }

    /// The same theme with `family` for code (`fontFamily` setting).
    #[must_use]
    pub fn with_mono_family(
        self,
        family: &'static str,
    ) -> Self {
        Self {
            mono_family: Some(family),
            ..self
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::App;

    use super::*;
    use crate::foundation::tokens::font::{HEADING_FAMILY, MONO_FAMILY, UI_FAMILY};

    #[test]
    fn ui_scale_is_size_over_fifteen_clamped() {
        const CASES: &[(f32, f32)] = &[(15.0, 1.0), (30.0, 1.6), (5.0, 10.0 / 15.0), (12.0, 0.8)];
        for &(size, scale) in CASES {
            let actual = Theme::default().with_ui_font_size(size).ui_scale();
            assert!((actual - scale).abs() < 1e-6, "size {size}: {actual}");
        }
    }

    #[gpui::test]
    fn installed_families_are_what_components_read(cx: &mut gpui::TestAppContext) {
        type Case = (Theme, &'static str, &'static str);
        let cases: [Case; 4] = [
            (Theme::default(), UI_FAMILY, HEADING_FAMILY),
            (
                Theme::default().with_ui_family("Inter"),
                "Inter",
                HEADING_FAMILY,
            ),
            (
                Theme::default().with_heading_family("Lora"),
                UI_FAMILY,
                "Lora",
            ),
            (
                Theme::default()
                    .with_ui_family("Inter")
                    .with_heading_family("Lora"),
                "Inter",
                "Lora",
            ),
        ];
        for (themed, ui, heading) in cases {
            let read = cx.update(|cx| installed(themed, cx));
            assert_eq!(read, (ui, heading));
        }
    }

    fn installed(
        themed: Theme,
        cx: &mut App,
    ) -> (&'static str, &'static str) {
        themed.install(cx);
        (Theme::of(cx).ui_family(), Theme::of(cx).heading_family())
    }

    #[gpui::test]
    fn installed_mono_family_is_what_components_read(cx: &mut gpui::TestAppContext) {
        type Case = (Theme, &'static str);
        let cases: [Case; 2] = [
            (Theme::default(), MONO_FAMILY),
            (Theme::default().with_mono_family("Fira Code"), "Fira Code"),
        ];
        for (themed, mono) in cases {
            cx.update(|cx| themed.install(cx));
            cx.update(|cx| assert_eq!(Theme::of(cx).mono_family(), mono));
        }
    }
}
