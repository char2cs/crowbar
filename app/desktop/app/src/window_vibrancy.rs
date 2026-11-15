use gpui::App;
use ui::{Theme, Vibrancy};

/// The backdrop the platform's windows give the chrome.
pub(crate) const fn of_platform() -> Vibrancy {
    // PLATFORM: only macOS windows blur what is behind them; X11 and Wayland windows are opaque.
    if cfg!(target_os = "macos") {
        Vibrancy::Translucent
    } else {
        Vibrancy::None
    }
}

/// Tells the installed theme how translucent the window is.
pub(crate) fn install(
    vibrancy: Vibrancy,
    cx: &mut App,
) {
    Theme::of(cx).with_vibrancy(vibrancy).install(cx);
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_installed_theme_carries_the_window_vibrancy(cx: &mut TestAppContext) {
        const CASES: &[Vibrancy] = &[Vibrancy::None, Vibrancy::Translucent];
        for &vibrancy in CASES {
            cx.update(|cx| {
                ui::init(cx).unwrap_or_else(|error| panic!("{error}"));
                install(vibrancy, cx);
                assert_eq!(Theme::of(cx), &Theme::of(cx).with_vibrancy(vibrancy));
            });
        }
    }
}
