//! How far an arrow key moves the separator.

use crate::metrics::{KEY_STEP, KEY_STEP_LARGE};

/// An arrow key moves 16px; with Shift, 64px.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum KeyStep {
    /// 16px.
    Fine,
    /// 64px.
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
#[path = "key_step_tests.rs"]
mod tests;
