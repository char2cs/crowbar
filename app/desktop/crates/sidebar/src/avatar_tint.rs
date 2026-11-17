//! The background colours of letter avatars.

use gpui::{Hsla, rgb};

/// Tailwind `*-700` families used by repository and project letter tiles.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum AvatarTint {
    /// Rose.
    Rose,
    /// Amber.
    Amber,
    /// Emerald.
    Emerald,
    /// Cyan.
    Cyan,
    /// Indigo.
    #[default]
    Indigo,
    /// Violet.
    Violet,
    /// Slate.
    Slate,
    /// Pink.
    Pink,
}

const ROSE: u32 = 0x00be_123c;
const AMBER: u32 = 0x00b4_5309;
const EMERALD: u32 = 0x0004_785f;
const CYAN: u32 = 0x000e_7490;
const INDIGO: u32 = 0x0043_38ca;
const VIOLET: u32 = 0x006d_28d9;
const SLATE: u32 = 0x0031_4158;
const PINK: u32 = 0x00be_185d;

impl AvatarTint {
    pub(crate) fn color(self) -> Hsla {
        let hex = match self {
            Self::Rose => ROSE,
            Self::Amber => AMBER,
            Self::Emerald => EMERALD,
            Self::Cyan => CYAN,
            Self::Indigo => INDIGO,
            Self::Violet => VIOLET,
            Self::Slate => SLATE,
            Self::Pink => PINK,
        };
        rgb(hex).into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const ALL: [AvatarTint; 8] = [
        AvatarTint::Rose,
        AvatarTint::Amber,
        AvatarTint::Emerald,
        AvatarTint::Cyan,
        AvatarTint::Indigo,
        AvatarTint::Violet,
        AvatarTint::Slate,
        AvatarTint::Pink,
    ];

    #[test]
    fn every_tint_is_opaque() {
        for tint in ALL {
            assert!((tint.color().a - 1.0).abs() < f32::EPSILON, "{tint:?}");
        }
    }

    #[test]
    fn every_tint_is_distinct() {
        let pairs = ALL
            .iter()
            .enumerate()
            .flat_map(|(index, tint)| ALL.iter().skip(index + 1).map(move |other| (tint, other)));
        for (tint, other) in pairs {
            assert_ne!(tint.color(), other.color(), "{tint:?} vs {other:?}");
        }
    }
}
