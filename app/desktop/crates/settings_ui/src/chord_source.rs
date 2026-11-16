//! Where the chord of a shortcut comes from.

/// Whether a shortcut shows its built-in chord or one the user set.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ChordSource {
    /// The default chord of the command.
    Default,
    /// The chord the active preset binds.
    Preset,
    /// A chord the user chose.
    Custom,
}
