use serde::{Deserialize, Serialize};

/// One element a pick found, as the page described it.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PointerItem {
    /// A selector that finds the element again.
    pub s: String,
    /// The tag name.
    pub tag: String,
    /// The ARIA role, or empty.
    pub role: String,
    /// The accessible name, or empty.
    pub name: String,
    /// The element's text, shortened.
    pub text: String,
    /// The bounding box: left, top, width, height.
    pub r: Vec<i64>,
    /// The element's markup, shortened.
    pub h: String,
    /// The reference number of the element in the page snapshot.
    pub i: i64,
}
