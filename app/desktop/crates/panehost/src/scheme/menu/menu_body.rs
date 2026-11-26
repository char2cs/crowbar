//! The body of a page's menu ask.

use serde::Deserialize;

use crate::scheme::menu::page_entry::PageEntry;

#[derive(Debug, Deserialize)]
pub(crate) struct MenuBody {
    pub(crate) entries: Vec<PageEntry>,
    pub(crate) x: f64,
    pub(crate) y: f64,
}
