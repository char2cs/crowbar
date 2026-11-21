//! Develop's selects and inputs are `text-base` below `sm` and `text-sm` above it, 4px taller below.

use gpui::{Pixels, Window, px};

use crate::foundation::breakpoint::Breakpoint;
use crate::foundation::tokens::text_size;

const GROWTH: f32 = 4.0;
const BASE_ICON: f32 = 18.0;
const SM_ICON: f32 = 16.0;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ControlScale {
    Base,
    Sm,
}

impl ControlScale {
    pub(crate) fn of(window: &Window) -> Self {
        if Breakpoint::Sm.is_cleared(window) {
            return Self::Sm;
        }
        Self::Base
    }

    pub(crate) fn growth(self) -> Pixels {
        match self {
            Self::Base => px(GROWTH),
            Self::Sm => px(0.0),
        }
    }

    pub(crate) fn text(self) -> Pixels {
        match self {
            Self::Base => text_size::BASE,
            Self::Sm => text_size::SM,
        }
    }

    pub(crate) fn icon(self) -> Pixels {
        match self {
            Self::Base => px(BASE_ICON),
            Self::Sm => px(SM_ICON),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn base_controls_are_taller_with_larger_text_and_icons() {
        let base = ControlScale::Base;
        let sm = ControlScale::Sm;
        assert_eq!(base.growth() - sm.growth(), px(4.0));
        assert_eq!((base.text(), sm.text()), (px(16.0), px(14.0)));
        assert_eq!((base.icon(), sm.icon()), (px(18.0), px(16.0)));
    }
}
