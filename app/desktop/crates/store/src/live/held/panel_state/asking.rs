//! Whether the followed chat's panels were asked of its daemon yet.

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) enum Asking {
    /// Nothing asked: the chat's host has not answered for it yet.
    #[default]
    NotYet,
    /// The listing and the changes were asked for.
    Sent,
}
