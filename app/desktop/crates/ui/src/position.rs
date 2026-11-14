//! Where an animated property is in its transition.

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Position {
    pub(crate) value: f32,
    pub(crate) running: bool,
}

impl Position {
    pub(crate) fn settled(value: f32) -> Self {
        Self {
            value,
            running: false,
        }
    }
}

#[cfg(test)]
#[path = "position_tests.rs"]
mod tests;
