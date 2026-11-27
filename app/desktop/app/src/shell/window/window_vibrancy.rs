use gpui::{App, Context, Window};
use ui::{Theme, Vibrancy};

pub(crate) const fn of_platform() -> Vibrancy {
    // PLATFORM: macos windows blur what is behind them; X11 and Wayland windows are opaque.
    if cfg!(target_os = "macos") {
        return Vibrancy::Translucent;
    }
    Vibrancy::None
}

pub(crate) fn install(
    vibrancy: Vibrancy,
    cx: &mut App,
) {
    Theme::of(cx).with_vibrancy(vibrancy).install(cx);
}

pub(crate) fn follow<V: 'static>(
    window: &mut Window,
    cx: &mut Context<'_, V>,
) {
    frost(window, cx);
    cx.observe_global_in::<Theme>(window, |_, window, cx| frost(window, cx))
        .detach();
}

// PLATFORM: macos only its windows blur what is behind them.
#[cfg(target_os = "macos")]
fn frost(
    window: &Window,
    cx: &App,
) {
    let Ok(webview::ParentWindow::AppKit(address)) = webview::ParentWindow::from_window(window)
    else {
        return;
    };
    let tone = match Theme::of(cx).appearance() {
        ui::Appearance::Light => platform::Frost::Light,
        ui::Appearance::Dark => platform::Frost::Dark,
    };
    if let Err(error) = platform::frost_window(address, tone) {
        tracing::warn!(%error, "the window stays unblurred");
    }
}

#[cfg(not(target_os = "macos"))]
fn frost(
    _window: &Window,
    _cx: &App,
) {
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    fn installed_carries(
        cx: &mut TestAppContext,
        vibrancy: Vibrancy,
    ) {
        cx.update(|cx| {
            ui::init(cx).unwrap_or_else(|error| panic!("{error}"));
            install(vibrancy, cx);
            assert_eq!(Theme::of(cx), &Theme::of(cx).with_vibrancy(vibrancy));
        });
    }

    #[gpui::test]
    fn the_installed_theme_carries_the_window_vibrancy(cx: &mut TestAppContext) {
        const CASES: &[Vibrancy] = &[Vibrancy::None, Vibrancy::Translucent];
        for &vibrancy in CASES {
            installed_carries(cx, vibrancy);
        }
    }
}
