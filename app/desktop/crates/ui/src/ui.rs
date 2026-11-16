//! Installation of the design system into a GPUI app.

use gpui::App;

use crate::appearance::Appearance;
use crate::error::Error;
use crate::font;
use crate::motion::Motion;
use crate::shimmer_clock::ShimmerClock;
use crate::theme::Theme;

/// Registers fonts, installs the default [`Theme`] and the [`Motion`] preference.
///
/// Call once at startup, before the first window opens, with the app built through
/// `Application::with_assets(ui::assets())` (or an asset source that wraps it).
///
/// # Errors
///
/// Returns [`Error::Fonts`] when an embedded font cannot be registered.
pub fn init(cx: &mut App) -> Result<(), Error> {
    font::register(cx)?;
    let system = Appearance::from(cx.window_appearance());
    cx.set_global(Theme::default().with_system(system));
    cx.set_global(Motion::default());
    cx.set_global(ShimmerClock::starting_now());
    cx.bind_keys(crate::menu_actions::bindings());
    cx.bind_keys(crate::popover_actions::bindings());
    cx.bind_keys(crate::text_input_actions::bindings());
    cx.bind_keys(crate::text_area_actions::bindings());
    Ok(())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::assets::assets;

    #[gpui::test]
    fn init_installs_the_globals_and_registers_fonts(cx: &mut TestAppContext) {
        cx.update(|cx| {
            assert!(init(cx).is_ok());
            assert!(cx.try_global::<Theme>().is_some());
            assert!(cx.try_global::<Motion>().is_some());
        });
        let _ = assets();
    }
}
