//! Layout and view-state model of the Crowbar shell.
//!
//! Pure data and operations, no GUI and no I/O. A [`Layout`] holds [`Tab`]s; a tab arranges
//! [`PaneInstance`]s in a binary split tree ([`SplitNode`]). Every change goes through
//! [`Layout::apply`] with a [`Command`], which either succeeds completely or changes
//! nothing, and [`LayoutHistory`] adds undo and redo of structural commands. The stored form
//! is versioned ([`SCHEMA_VERSION`]) and older versions are migrated on load.
//!
//! ```
//! use layout::{Command, Layout, PaneInstance, TabId};
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

mod arrangement;
mod axis;
mod branch;
mod close_panes;
mod close_scope;
mod command;
mod division;
mod drag_subject;
mod edge;
mod error;
mod group;
mod group_commands;
mod group_pins;
mod group_slot;
mod layout;
mod layout_document;
mod layout_execute;
mod layout_history;
mod layout_stored;
mod layout_tabs;
mod model;
mod pane_commands;
mod pane_instance;
mod pane_slot;
mod pin_commands;
mod placement;
mod rect;
mod remaining;
mod sash;
mod schema;
mod split_extent;
mod split_node;
mod split_node_edit;
mod split_node_place;
mod strip_step;
mod tab;
mod tab_commands;
mod tab_edit;
mod tab_groups;
mod tab_id;
mod tab_open;
mod tab_record;
mod tab_shape;
mod tab_view;
#[cfg(test)]
mod test_support;
mod undo_step;

pub use axis::Axis;
pub use branch::Branch;
pub use close_scope::CloseScope;
pub use command::Command;
pub use drag_subject::DragSubject;
pub use edge::Edge;
pub use error::{Error, Result};
pub use group::Group;
pub use group_slot::GroupSlot;
pub use layout::{Layout, MAX_TABS};
pub use layout_history::{LayoutHistory, MAX_PARKED_PANES, MAX_UNDO_STEPS};
pub use model::{load, store};
pub use pane_instance::PaneInstance;
pub use pane_slot::PaneSlot;
pub use placement::Placement;
pub use rect::Rect;
pub use sash::Sash;
pub use schema::SCHEMA_VERSION;
pub use split_node::{MIN_PANE_EXTENT, SASH_THICKNESS, SplitNode};
pub use strip_step::StripStep;
pub use tab::{MAX_PANES_PER_TAB, Tab};
pub use tab_id::TabId;
