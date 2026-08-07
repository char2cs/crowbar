//! The shipping window's content: the sidebar, the splitter, and the pane
//! the later slices fill.
//!
//! Slice 1a. Replaces S0.2's [`super::Placeholder`], which drew the daemon
//! caption and nothing else.

pub mod coordinator;
pub mod model;
pub mod root;
pub mod sidebar;

pub use root::Shell;
pub use sidebar::Sidebar;
