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

impl AvatarTint {
    pub(crate) fn color(self) -> Hsla {
        let hex = match self {
            Self::Rose => 0x00be_123c,
            Self::Amber => 0x00b4_5309,
            Self::Emerald => 0x0004_785f,
            Self::Cyan => 0x000e_7490,
            Self::Indigo => 0x0043_38ca,
            Self::Violet => 0x006d_28d9,
            Self::Slate => 0x0033_4155,
            Self::Pink => 0x00be_185d,
        };
        rgb(hex).into()
    }
}

#[cfg(test)]
#[path = "avatar_tint_tests.rs"]
mod tests;
