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

type Flag = bool;

impl From<Flag> for Pose {
    fn from(on: Flag) -> Self {
        if on {
            return Self::On;
        }
        Self::Off
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
mod tests {
    use super::*;

    #[test]
    fn converts_both_ways_with_bool() {
        assert_eq!(Pose::from(true), Pose::On);
        assert_eq!(Pose::from(false), Pose::Off);
        assert!(bool::from(Pose::On));
        assert!(!bool::from(Pose::Off));
    }

    #[test]
    fn rest_value_is_zero_or_one() {
        assert!((Pose::Off.rest() - 0.0).abs() < f32::EPSILON);
        assert!((Pose::On.rest() - 1.0).abs() < f32::EPSILON);
    }
}
