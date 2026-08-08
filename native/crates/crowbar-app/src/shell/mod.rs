//! The shipping window's content: the sidebar, the splitter, and the pane
//! the later slices fill.
//!
//! Slice 1a. Replaces S0.2's [`super::Placeholder`], which drew the daemon
//! caption and nothing else.

#[cfg(test)]
pub mod compare;
pub mod coordinator;
pub mod model;
pub mod root;
#[cfg(test)]
pub mod screenshot;
pub mod sidebar;

pub use root::Shell;
pub use sidebar::Sidebar;
