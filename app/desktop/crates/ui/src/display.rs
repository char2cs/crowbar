//! Components that show state: scroll areas, badges, skeletons, toasts, keys, spinners and
//! tracked text.

mod badge;
mod flicker_spinner;
mod kbd;
mod scroll_area;
mod skeleton;
mod spinner;
mod toast;
mod tracked_text;

pub use badge::{Badge, BadgeSize, BadgeVariant};
pub use flicker_spinner::FlickerSpinner;
pub use kbd::Kbd;
pub use scroll_area::ScrollArea;
pub(crate) use skeleton::ShimmerClock;
pub use skeleton::Skeleton;
pub use spinner::Spinner;
pub use toast::{
    Toast, ToastContent, ToastId, ToastKind, ToastLifetime, ToastPlacement, ToastQueue,
    ToastQueueEvent, ToastSide,
};
pub use tracked_text::TrackedText;
