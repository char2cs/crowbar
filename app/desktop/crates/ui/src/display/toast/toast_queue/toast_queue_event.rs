//! Events a toast queue emits.

use crate::display::toast::toast_queue::toast_id::ToastId;

/// What happened in a [`crate::ToastQueue`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ToastQueueEvent {
    /// A toast finished leaving, whether it expired or the user dismissed it.
    Removed(ToastId),
}
