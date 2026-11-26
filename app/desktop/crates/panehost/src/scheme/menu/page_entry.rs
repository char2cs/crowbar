//! One menu entry as a page writes it.

use serde::Deserialize;

#[derive(Debug, Deserialize)]
#[serde(tag = "kind", rename_all = "lowercase")]
pub(crate) enum PageEntry {
    Item {
        label: String,
        enabled: bool,
    },
    Submenu {
        label: String,
        enabled: bool,
        entries: Vec<PageEntry>,
    },
    Separator,
}
