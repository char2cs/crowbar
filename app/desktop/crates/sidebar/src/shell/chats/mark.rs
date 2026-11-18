//! The picture of a repository or space.

mod avatar_tint;

pub use avatar_tint::AvatarTint;

use std::path::PathBuf;

/// A letter tile, an emoji, an image or the default books glyph.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Mark {
    /// The first letter of the name on a tinted tile.
    Letter {
        /// The letter shown.
        letter: char,
        /// The tile colour.
        tint: AvatarTint,
    },
    /// An emoji avatar.
    Emoji(String),
    /// An image file.
    Image(PathBuf),
    /// No mark chosen: the books glyph.
    Fallback,
}
