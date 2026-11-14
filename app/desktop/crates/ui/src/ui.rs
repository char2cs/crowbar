//! Installation of the design system into a GPUI app.

use gpui::App;

use crate::appearance::Appearance;
use crate::error::Error;
use crate::font;
use crate::motion::Motion;
use crate::theme::Theme;

/// Registers fonts, installs the default [`Theme`] and the [`Motion`] preference.
///
/// Call once at startup, before the first window opens, with the app built through
/// `Application::with_assets(ui::Assets)` (or an asset source that wraps it).
///
/// # Errors
///
/// Returns [`Error::Fonts`] when an embedded font cannot be registered.
pub fn init(cx: &mut App) -> Result<(), Error> {
    font::register(cx)?;
    let system = Appearance::from(cx.window_appearance());
    cx.set_global(Theme::default().with_system(system));
    cx.set_global(Motion::default());
    cx.bind_keys(crate::menu_actions::bindings());
    cx.bind_keys(crate::text_input_actions::bindings());
    Ok(())
}

#[cfg(test)]
#[path = "ui_tests.rs"]
mod tests;
