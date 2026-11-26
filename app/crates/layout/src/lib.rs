//! Layout and view-state model of the Crowbar shell.
//!
//! Pure data and operations, no GUI and no I/O. A [`Layout`] holds [`Tab`]s; a tab arranges
//! [`PaneInstance`]s in a binary split tree ([`SplitNode`]). Every change goes through
//! [`Layout::apply`] with a [`Command`], which either succeeds completely or changes
//! nothing, and [`LayoutHistory`] adds undo and redo of structural commands. The stored form
//! is versioned and older versions are migrated on load.
//!
//! ```
//! use layout::{Command, Layout, PaneInstance, SpaceId, TabId};
//! use protocol::{PaneId, PaneState, PaneTypeId, Params};
//!
//! let mut layout = Layout::default();
//! layout.apply(Command::AddTab { id: TabId(1), title: "Chat".into(), space: SpaceId::default() })?;
//! layout.apply(Command::OpenPane {
//!     tab: TabId(1),
//!     instance: PaneInstance {
//!         pane_id: PaneId::new("p1")?,
//!         type_id: PaneTypeId::new("core.chat")?,
//!         params: Params::default(),
//!         state: PaneState::default(),
//!     },
//!     placement: None,
//! })?;
//! assert_eq!(layout.showing("").map(|tab| tab.pane_ids().len()), Some(1));
//! # Ok::<(), Box<dyn std::error::Error>>(())
//! ```

mod drag_subject;
mod error;
mod ide;
mod layout;
mod space_id;
mod tab_id;
#[cfg(test)]
mod test_support;
mod tree;

pub use drag_subject::DragSubject;
pub use error::{Error, Result};
pub use ide::pane_group::PaneGroup;
pub use ide::pane_group::close_scope::CloseScope;
pub use ide::pane_group::strip_step::StripStep;
pub use ide::presentation::editor_view::EditorView;
pub use ide::presentation::sidebar_side::SidebarSide;
pub use ide::presentation::{DEFAULT_CHAT_SHARE, Presentation, TOP_ROW_HEIGHT};
pub use layout::command::Command;
pub use layout::layout_history::LayoutHistory;
pub use layout::stored::layout_stored::{load, store};
pub use layout::tab::pane_instance::PaneInstance;
pub use layout::tab::Tab;
pub use layout::Layout;
pub use space_id::SpaceId;
pub use tab_id::TabId;
pub use tree::axis::Axis;
pub use tree::branch::Branch;
pub use tree::edge::Edge;
pub use tree::geometry::pane_slot::PaneSlot;
pub use tree::geometry::rect::Rect;
pub use tree::geometry::sash::Sash;
pub use tree::placement::Placement;
pub use tree::split_node::SplitNode;
