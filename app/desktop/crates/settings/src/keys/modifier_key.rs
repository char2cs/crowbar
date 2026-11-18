//! A modifier key held during a key press.

use chord::Primary;

/// One of the modifier keys.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ModifierKey {
    /// Control.
    Control,
    /// Alt or Option.
    Alt,
    /// Shift.
    Shift,
    /// Command on macOS, the logo key elsewhere.
    Platform,
}

impl ModifierKey {
    /// The physical key that plays `mod` on a platform whose primary modifier is `primary`.
    pub fn primary(primary: Primary) -> Self {
        match primary {
            Primary::Control => Self::Control,
            Primary::Platform => Self::Platform,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_primary_modifier_is_the_key_that_plays_mod() {
        let cases = [
            (Primary::Control, ModifierKey::Control),
            (Primary::Platform, ModifierKey::Platform),
        ];
        for (primary, key) in cases {
            assert_eq!(ModifierKey::primary(primary), key, "{primary:?}");
        }
    }
}
