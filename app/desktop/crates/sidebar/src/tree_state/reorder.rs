//! Drag rules of the chats tree: when a press becomes a drag, edge scrolling, drop modes, the
//! drop policy and folder targets.

mod drag_arm;
mod drop_modes;
mod drop_policy;
mod edge_scroll;
mod folder_target;

pub use drag_arm::drag_started;
pub use drop_modes::DropModes;
pub use edge_scroll::pull;
pub use folder_target::FolderTarget;
