use serde::{Deserialize, Serialize};

const SM: f32 = 640.0;
const MD: f32 = 768.0;
const LG: f32 = 1024.0;
const XL: f32 = 1280.0;

/// A Tailwind breakpoint (`sm`, `md`, `lg`, `xl`), cleared when the app window is at least as
/// wide. Develop renders in one webview, so its breakpoints ask about the window; the shell
/// draws by them and tells each pane which ones the window clears.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Breakpoint {
    /// 640 px.
    Sm,
    /// 768 px.
    Md,
    /// 1024 px.
    Lg,
    /// 1280 px.
    Xl,
}

impl Breakpoint {
    /// Every breakpoint, narrowest first.
    pub const ALL: [Self; 4] = [Self::Sm, Self::Md, Self::Lg, Self::Xl];

    /// The narrowest window width, in logical pixels, that clears this breakpoint.
    #[must_use]
    pub fn min_width(self) -> f32 {
        match self {
            Self::Sm => SM,
            Self::Md => MD,
            Self::Lg => LG,
            Self::Xl => XL,
        }
    }

    /// The breakpoints a window `width` logical pixels wide clears, narrowest first.
    #[must_use]
    pub fn cleared_by(width: f32) -> Vec<Self> {
        Self::ALL
            .into_iter()
            .filter(|point| width >= point.min_width())
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::Breakpoint::{self, Lg, Md, Sm, Xl};

    #[test]
    fn a_width_clears_every_breakpoint_at_or_below_it() {
        let cases: [(f32, &[Breakpoint]); 5] = [
            (639.0, &[]),
            (640.0, &[Sm]),
            (900.0, &[Sm, Md]),
            (1024.0, &[Sm, Md, Lg]),
            (1600.0, &[Sm, Md, Lg, Xl]),
        ];
        for (width, cleared) in cases {
            assert_eq!(Breakpoint::cleared_by(width), cleared, "{width}");
        }
    }
}
