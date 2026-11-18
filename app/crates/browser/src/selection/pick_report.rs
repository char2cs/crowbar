use serde::{Deserialize, Serialize};

use crate::selection::pointer_item::PointerItem;

/// What the page returns for a finished pick.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PickReport {
    /// `element`, `region`, `text` or `multi`.
    pub kind: String,
    /// The page title.
    pub title: String,
    /// The page address.
    pub url: String,
    /// The picked elements, at most ten.
    pub items: Vec<PointerItem>,
    /// How many more were found beyond `items`.
    pub more: usize,
    /// The bounding box of the pick.
    #[serde(rename = "box")]
    pub bounds: Vec<i64>,
    /// The selected text, for a `text` pick.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub quote: Option<String>,
}
