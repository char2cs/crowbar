//! What can stop a queue's toasts from expiring.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ToastHold {
    Pointer,
    Window,
}
