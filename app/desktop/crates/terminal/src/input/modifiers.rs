//! Keyboard modifier state.

const CONTROL_WEIGHT: u8 = 4;

/// The modifier keys held during an input event.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Modifiers(u8);

impl Modifiers {
    /// Nothing held.
    pub const NONE: Self = Self(0);
    /// Shift.
    pub const SHIFT: Self = Self(1);
    /// Alt or Option.
    pub const ALT: Self = Self(1 << 1);
    /// Control.
    pub const CONTROL: Self = Self(1 << 2);
    /// Command or the Windows/Super key.
    pub const PLATFORM: Self = Self(1 << 3);

    /// True when every modifier of `other` is held.
    pub fn contains(
        self,
        other: Self,
    ) -> bool {
        self.0 & other.0 == other.0
    }

    /// The union of both sets.
    #[must_use]
    pub fn with(
        self,
        other: Self,
    ) -> Self {
        Self(self.0 | other.0)
    }

    /// The xterm modifier parameter: 1 plus shift 1, alt 2 and control 4; 1 means none.
    pub fn xterm_parameter(self) -> u8 {
        let shift = u8::from(self.contains(Self::SHIFT));
        let alt = u8::from(self.contains(Self::ALT)) * 2;
        let control = u8::from(self.contains(Self::CONTROL)) * CONTROL_WEIGHT;
        1 + shift + alt + control
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn xterm_parameter_sums_the_held_modifiers() {
        let cases = [
            (Modifiers::NONE, 1),
            (Modifiers::SHIFT, 2),
            (Modifiers::ALT, 3),
            (Modifiers::CONTROL, 5),
            (Modifiers::SHIFT.with(Modifiers::CONTROL), 6),
            (Modifiers::PLATFORM, 1),
        ];
        for (modifiers, expected) in cases {
            assert_eq!(modifiers.xterm_parameter(), expected, "{modifiers:?}");
        }
    }
}
