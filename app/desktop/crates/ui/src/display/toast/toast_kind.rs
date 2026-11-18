//! Tone of a toast.

/// What a [`crate::Toast`] reports.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ToastKind {
    /// Neutral information.
    #[default]
    Info,
    /// An action finished.
    Success,
    /// Something needs attention.
    Warning,
    /// An action failed; the toast shakes once.
    Error,
    /// Work in progress with a spinner.
    Loading,
}
