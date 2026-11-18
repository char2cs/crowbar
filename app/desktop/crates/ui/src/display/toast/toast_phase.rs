//! Whether a toast is appearing or going away.

/// The fade a [`crate::Toast`] plays.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ToastPhase {
    /// Fades in; the state of every toast that is shown.
    #[default]
    Shown,
    /// Fades out; the owner removes the toast once the fade is done.
    Leaving,
}
