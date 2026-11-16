use gpui::App;
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
