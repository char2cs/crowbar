//! How far an arrow key moves the separator.

use crate::metrics::{KEY_STEP, KEY_STEP_LARGE};

/// The size of one arrow-key move of the separator.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum KeyStep {
    /// A plain arrow key: a small step.
    Fine,
    /// An arrow key with Shift: four times the fine step.
    Coarse,
}

impl KeyStep {
    pub(crate) fn pixels(self) -> f32 {
        match self {
            Self::Fine => KEY_STEP,
            Self::Coarse => KEY_STEP_LARGE,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(KeyStep, f32); 2] = [(KeyStep::Fine, 16.0), (KeyStep::Coarse, 64.0)];

    #[test]
    fn shift_quadruples_the_step() {
        for (step, pixels) in CASES {
            assert!((step.pixels() - pixels).abs() < f32::EPSILON, "{step:?}");
        }
    }
}
