use protocol::PaneId;
use thiserror::Error;

use crate::tab_id::TabId;

/// A layout command was refused. A refused command leaves the layout unchanged.
#[derive(Debug, Clone, PartialEq, Eq, Error)]
#[non_exhaustive]
pub enum Error {
    /// No tab has this id.
    #[error("no tab {0:?}")]
    TabNotFound(TabId),
    /// A tab with this id already exists.
    #[error("tab {0:?} already exists")]
    DuplicateTab(TabId),
    /// No pane has this id.
    #[error("no pane {0}")]
    PaneNotFound(PaneId),
    /// A pane with this id already exists in the layout.
    #[error("pane {0} already exists")]
    DuplicatePane(PaneId),
    /// The command needs two different panes.
    #[error("pane {0} cannot be placed relative to itself")]
    SamePane(PaneId),
    /// The command needs two different tabs.
    #[error("tab {0:?} is both source and destination")]
    SameTab(TabId),
    /// A placement is needed because the tab already has panes and none is focused.
    #[error("tab {0:?} has panes but no focused pane to place next to")]
    PlacementRequired(TabId),
    /// The pane is the only chat of its tab, so it cannot be detached from its siblings.
    #[error("pane {0} is the only chat of its tab")]
    OnlyPane(PaneId),
    /// The pane is an editor tab, which leaves a tab only with its chat.
    #[error("pane {0} is an editor tab, not a chat")]
    NotAChat(PaneId),
    /// The pane is a chat, which is never an editor tab: it cannot join a group's tabs or swap
    /// with one.
    #[error("pane {0} is a chat, not an editor tab")]
    ChatIsNotATab(PaneId),
    /// An index is outside the list it addresses.
    #[error("index {index} is out of range for {len} items")]
    IndexOutOfRange {
        /// The requested index.
        index: usize,
        /// The number of items.
        len: usize,
    },
    /// A split path does not lead to a split.
    #[error("the path does not address a split")]
    NoSuchSplit,
    /// A size or rectangle is not a finite, non-negative number.
    #[error("sizes must be finite, non-negative numbers")]
    InvalidGeometry,
    /// The layout already holds as many tabs as allowed.
    #[error("a layout holds at most {limit} tabs")]
    TooManyTabs {
        /// The limit.
        limit: usize,
    },
    /// The tab already holds as many panes as allowed.
    #[error("tab {tab:?} holds at most {limit} panes")]
    TooManyPanes {
        /// The tab that is full.
        tab: TabId,
        /// The limit.
        limit: usize,
    },
    /// Stored layout data is not readable JSON of the expected shape.
    #[error("cannot read the stored layout: {0}")]
    Unreadable(String),
    /// Loaded data breaks a layout invariant.
    #[error("invalid layout: {0}")]
    Invalid(&'static str),
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
