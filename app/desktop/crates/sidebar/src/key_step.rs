//! How far an arrow key moves the separator.

use crate::metrics::{KEY_STEP, KEY_STEP_LARGE};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum KeyStep {
    Fine,
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
