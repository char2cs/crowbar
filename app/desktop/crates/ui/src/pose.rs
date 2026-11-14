//! The two values of a toggled property.

/// Off or on; stands in for a `bool` where a function must not take one.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Pose {
    /// Hovered-out, unchecked.
    #[default]
    Off,
    /// Hovered, checked.
    On,
}

impl From<bool> for Pose {
    fn from(on: bool) -> Self {
        if on { Self::On } else { Self::Off }
    }
}

impl From<Pose> for bool {
    fn from(pose: Pose) -> Self {
        pose == Pose::On
    }
}

impl Pose {
    pub(crate) fn rest(self) -> f32 {
        f32::from(u8::from(self == Self::On))
    }
}

#[cfg(test)]
#[path = "pose_tests.rs"]
mod tests;
