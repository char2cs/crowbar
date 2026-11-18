//! Identity of a toast in a queue.

/// Names one toast of a [`crate::ToastQueue`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct ToastId(pub(crate) u64);
