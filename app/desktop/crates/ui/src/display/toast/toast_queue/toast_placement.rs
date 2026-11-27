pub(super) mod toast_side;

use crate::display::toast::toast_queue::toast_placement::toast_side::ToastSide;

const DOCKED_LIMIT: usize = 3;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ToastPlacement {
    Docked(ToastSide),
    Floating(ToastSide),
}

impl Default for ToastPlacement {
    fn default() -> Self {
        Self::Docked(ToastSide::default())
    }
}

impl ToastPlacement {
    pub(crate) fn side(self) -> ToastSide {
        match self {
            Self::Docked(side) | Self::Floating(side) => side,
        }
    }

    pub(crate) fn limit(self) -> Option<usize> {
        match self {
            Self::Docked(_) => Some(DOCKED_LIMIT),
            Self::Floating(_) => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn docked_columns_cap_the_toasts_and_floating_ones_do_not() {
        type Case = (ToastPlacement, ToastSide, Option<usize>);
        const CASES: &[Case] = &[
            (
                ToastPlacement::Docked(ToastSide::Left),
                ToastSide::Left,
                Some(3),
            ),
            (
                ToastPlacement::Floating(ToastSide::Right),
                ToastSide::Right,
                None,
            ),
        ];
        for &(placement, side, limit) in CASES {
            assert_eq!((placement.side(), placement.limit()), (side, limit));
        }
    }
}
