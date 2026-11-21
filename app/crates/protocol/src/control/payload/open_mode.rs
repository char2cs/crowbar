use serde::{Deserialize, Serialize};

/// How a pane asked to be opened is kept, as develop's `openDocument` takes `preview`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum OpenMode {
    /// An ordinary tab.
    #[default]
    Kept,
    /// The group's preview tab, written in italics until it is kept.
    Preview,
}
