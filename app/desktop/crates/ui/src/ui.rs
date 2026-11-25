//! Installation of the design system into a GPUI app.

use gpui::App;

use crate::display::ShimmerClock;
use crate::error::Error;
use crate::foundation::Appearance;
use crate::foundation::Motion;
use crate::foundation::Theme;
use crate::foundation::tokens::font;

/// Registers fonts, installs the default [`Theme`] and the [`Motion`] preference, and draws
/// text in grayscale, as WebKit does, not with gpui's colour-fringed subpixel default on Linux.
///
/// Call once at startup, before the first window opens, with the app built through
/// `Application::with_assets(ui::assets())` (or an asset source that wraps it).
///
/// # Errors
///
/// Returns [`Error::Fonts`] when an embedded font cannot be registered.
pub fn init(cx: &mut App) -> Result<(), Error> {
    font::register(cx)?;
    cx.set_text_rendering_mode(gpui::TextRenderingMode::Grayscale);
    let system = Appearance::from(cx.window_appearance());
    cx.set_global(Theme::default().with_system(system));
    cx.set_global(Motion::default());
    crate::foundation::Modality::install(cx);
    cx.set_global(ShimmerClock::starting_now());
    cx.bind_keys(crate::floating::menu_bindings());
    cx.bind_keys(crate::floating::popover_bindings());
    cx.bind_keys(crate::floating::dialog_bindings());
    cx.bind_keys(crate::control::text_input_bindings());
    cx.bind_keys(crate::control::text_area_bindings());
    Ok(())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::foundation::assets;

    #[gpui::test]
    fn init_installs_the_globals_and_registers_fonts(cx: &mut TestAppContext) {
        cx.update(|cx| {
            assert!(init(cx).is_ok());
            assert!(cx.try_global::<Theme>().is_some());
            assert!(cx.try_global::<Motion>().is_some());
        });
        let _ = assets();
    }

    #[gpui::test]
    fn init_draws_text_in_grayscale(cx: &mut TestAppContext) {
        cx.update(|cx| {
            assert!(init(cx).is_ok());
            assert_eq!(cx.text_rendering_mode(), gpui::TextRenderingMode::Grayscale);
        });
    }
}
