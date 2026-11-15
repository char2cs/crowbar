//! The kind of chat a create starts.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Surface {
    /// The user's default.
    Default,
    /// An agent running in a terminal.
    Terminal,
}
