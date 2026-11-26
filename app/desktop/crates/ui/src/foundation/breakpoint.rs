//! Tailwind's breakpoints, asked of the app window as develop's single webview asks them.

use gpui::{Window, px};

const SM: f32 = 640.0;
const MD: f32 = 768.0;
const LG: f32 = 1024.0;
const XL: f32 = 1280.0;

/// A Tailwind breakpoint (`sm`, `md`, `lg`, `xl`), cleared when the app window is at least as wide.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Breakpoint {
    /// 640px.
    Sm,
    /// 768px.
    Md,
    /// 1024px.
    Lg,
    /// 1280px.
    Xl,
}

impl Breakpoint {
    /// Every breakpoint, narrowest first.
    pub const ALL: [Self; 4] = [Self::Sm, Self::Md, Self::Lg, Self::Xl];

    /// The narrowest window width that clears this breakpoint.
    pub fn min_width(self) -> f32 {
        match self {
            Self::Sm => SM,
            Self::Md => MD,
            Self::Lg => LG,
            Self::Xl => XL,
        }
    }

    /// The breakpoints a window `width` pixels wide clears.
    pub fn cleared_by(width: f32) -> Vec<Self> {
        Self::ALL
            .into_iter()
            .filter(|point| width >= point.min_width())
            .collect()
    }

    /// Whether the app window `window` belongs to clears this breakpoint.
    pub fn is_cleared(
        self,
        window: &Window,
    ) -> bool {
        app_window_width(window) >= self.min_width()
    }
}

/// The width of the app window, which dialogs and popups painted in it follow.
pub fn app_window_width(window: &Window) -> f32 {
    f32::from(window.viewport_size().width.max(px(0.0)))
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, size};

    use super::*;
    use crate::test_support::probe::open;

    #[test]
    fn a_width_clears_each_threshold_it_reaches() {
        let cases: [(f32, &[Breakpoint]); 5] = [
            (639.0, &[]),
            (640.0, &[Breakpoint::Sm]),
            (800.0, &[Breakpoint::Sm, Breakpoint::Md]),
            (1024.0, &[Breakpoint::Sm, Breakpoint::Md, Breakpoint::Lg]),
            (1280.0, &Breakpoint::ALL),
        ];
        for (width, cleared) in cases {
            assert_eq!(Breakpoint::cleared_by(width), cleared, "{width}");
        }
    }

    #[gpui::test]
    fn a_plain_window_answers_with_its_own_width(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| gpui::div());
        cx.simulate_resize(size(px(600.0), px(400.0)));
        cx.update(|window, _| {
            assert_eq!(app_window_width(window), 600.0);
            assert!(!Breakpoint::Sm.is_cleared(window));
        });
    }
}
