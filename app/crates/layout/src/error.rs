use protocol::PaneId;
use thiserror::Error;

use crate::tab_id::TabId;

#[derive(Debug, Clone, PartialEq, Eq, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("no tab {0:?}")]
    TabNotFound(TabId),
    #[error("tab {0:?} already exists")]
    DuplicateTab(TabId),
    #[error("no pane {0}")]
    PaneNotFound(PaneId),
    #[error("pane {0} already exists")]
    DuplicatePane(PaneId),
    #[error("pane {0} cannot be placed relative to itself")]
    SamePane(PaneId),
    #[error("tab {0:?} is both source and destination")]
    SameTab(TabId),
    #[error("tab {0:?} has panes but no focused pane to place next to")]
    PlacementRequired(TabId),
    #[error("pane {0} is the only chat of its tab")]
    OnlyPane(PaneId),
    #[error("pane {0} is an editor tab, not a chat")]
    NotAChat(PaneId),
    #[error("pane {0} is a chat, not an editor tab")]
    ChatIsNotATab(PaneId),
    #[error("index {index} is out of range for {len} items")]
    IndexOutOfRange { index: usize, len: usize },
    #[error("the path does not address a split")]
    NoSuchSplit,
    #[error("sizes must be finite, non-negative numbers")]
    InvalidGeometry,
    #[error("a layout holds at most {limit} tabs")]
    TooManyTabs { limit: usize },
    #[error("tab {tab:?} holds at most {limit} panes")]
    TooManyPanes { tab: TabId, limit: usize },
    #[error("tab {0:?} already belongs to a space")]
    Owned(TabId),
    #[error("cannot read the stored layout: {0}")]
    Unreadable(String),
    #[error("invalid layout: {0}")]
    Invalid(&'static str),
}

pub type Result<T> = std::result::Result<T, Error>;
