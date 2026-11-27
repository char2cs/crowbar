use crate::display::toast::toast_queue::toast_id::ToastId;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ToastQueueEvent {
    Removed(ToastId),
}
