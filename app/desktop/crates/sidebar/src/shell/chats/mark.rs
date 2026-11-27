mod avatar_tint;

pub use avatar_tint::AvatarTint;

use std::path::PathBuf;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Mark {
    Letter { letter: char, tint: AvatarTint },
    Emoji(String),
    Image(PathBuf),
    Fallback,
}
