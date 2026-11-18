//! The background colours of letter avatars.

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
