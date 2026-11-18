//! The colour meaning of a status.

/// How a status reads: neutral text, or a colour that carries meaning.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Tone {
    /// Plain muted text.
    #[default]
    Neutral,
    /// Online, installed, healthy.
    Success,
    /// Needs attention.
    Warning,
    /// Offline, blocked, failed.
    Error,
    /// Not settled yet.
    Pending,
}
