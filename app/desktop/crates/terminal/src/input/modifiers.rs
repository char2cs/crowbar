const CONTROL_WEIGHT: u8 = 4;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Modifiers(u8);

impl Modifiers {
    pub const NONE: Self = Self(0);
    pub const SHIFT: Self = Self(1);
    pub const ALT: Self = Self(1 << 1);
    pub const CONTROL: Self = Self(1 << 2);
    pub const PLATFORM: Self = Self(1 << 3);

    pub fn contains(
        self,
        other: Self,
    ) -> bool {
        self.0 & other.0 == other.0
    }

    #[must_use]
    pub fn with(
        self,
        other: Self,
    ) -> Self {
        Self(self.0 | other.0)
    }

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
