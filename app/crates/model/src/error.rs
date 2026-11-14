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
    /// A placement is needed because the tab already has panes and none is focused.
    #[error("tab {0:?} has panes but no focused pane to place next to")]
    PlacementRequired(TabId),
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
    /// A ratio is not a finite number.
    #[error("split ratio must be a finite number")]
    InvalidRatio,
    /// Loaded data breaks a layout invariant.
    #[error("invalid layout: {0}")]
    Invalid(&'static str),
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
