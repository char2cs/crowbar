use serde::{Deserialize, Serialize};

use crate::selection::pointer_item::PointerItem;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PickReport {
    pub kind: String,
    pub title: String,
    pub url: String,
    pub items: Vec<PointerItem>,
    pub more: usize,
    #[serde(rename = "box")]
    pub bounds: Vec<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub quote: Option<String>,
}
