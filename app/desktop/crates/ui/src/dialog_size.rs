//! Width steps of an application dialog.

use gpui::{Pixels, px};

const WIDTH_SM: Pixels = px(384.0);
const WIDTH_MD: Pixels = px(448.0);
const WIDTH_LG: Pixels = px(672.0);
const WIDTH_XL: Pixels = px(896.0);

/// Maximum width of a [`crate::Dialog`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum DialogSize {
    /// 384px.
    Sm,
    /// 448px.
    #[default]
    Md,
    /// 672px.
    Lg,
    /// 896px.
    Xl,
}

impl DialogSize {
    /// The maximum popup width.
    pub fn max_width(self) -> Pixels {
        match self {
            Self::Sm => WIDTH_SM,
            Self::Md => WIDTH_MD,
            Self::Lg => WIDTH_LG,
            Self::Xl => WIDTH_XL,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn widths_match_the_spec() {
        const CASES: &[(DialogSize, f32)] = &[
            (DialogSize::Sm, 384.0),
            (DialogSize::Md, 448.0),
            (DialogSize::Lg, 672.0),
            (DialogSize::Xl, 896.0),
        ];
        for &(size, width) in CASES {
            assert_eq!(f32::from(size.max_width()), width, "{size:?}");
        }
    }
}
