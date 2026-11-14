//! Layout and view-state model of the Crowbar shell.
//!
//! Pure data and operations, no GUI and no I/O. A [`Layout`] holds [`Tab`]s; a tab arranges
//! [`PaneInstance`]s in a binary split tree ([`SplitNode`]). Every change goes through
//! [`Layout::apply`] with a [`Command`], which either succeeds completely or changes
//! nothing, and [`LayoutHistory`] adds undo and redo of structural commands.
//!
//! ```
//! use model::{Command, Layout, PaneInstance, TabId};
//! use protocol::{PaneId, PaneState, PaneTypeId, Params};
//!
//! let mut layout = Layout::default();
//! layout.apply(Command::AddTab { id: TabId(1), title: "Chat".into() })?;
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
//! assert_eq!(layout.active_tab().map(|tab| tab.pane_ids().len()), Some(1));
//! # Ok::<(), Box<dyn std::error::Error>>(())
//! ```

mod axis;
mod branch;
mod command;
mod edge;
mod error;
mod layout;
mod layout_history;
mod pane_instance;
mod pane_slot;
mod placement;
mod rect;
mod split_node;
mod tab;
mod tab_id;

pub use axis::Axis;
pub use branch::Branch;
pub use command::Command;
pub use edge::Edge;
pub use error::{Error, Result};
pub use layout::Layout;
pub use layout_history::LayoutHistory;
pub use pane_instance::PaneInstance;
pub use pane_slot::PaneSlot;
pub use placement::Placement;
pub use rect::Rect;
pub use split_node::{MIN_RATIO, SplitNode};
pub use tab::Tab;
pub use tab_id::TabId;
