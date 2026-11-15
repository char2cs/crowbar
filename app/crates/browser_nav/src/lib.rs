//! Navigation logic of the browser pane (`core.browser`).
//!
//! A browser pane loads arbitrary web pages, so it is untrusted by construction: it gets no
//! relay token, no `crowbar://` scheme and no pane protocol (the shell enforces that when it
//! picks the web view kind). This crate holds what the native chrome needs and nothing a page
//! can influence beyond display: [`resolve_address`] turns typed text into a URL,
//! [`BrowserUrl`] is the only URL form a browser pane loads, [`PageUpdate`] is the engine's
//! report of what happened in the page and [`PageState`] folds those reports into what the
//! address bar, title and reload button show.

mod browser_nav;
mod browser_url;
mod error;
mod page_state;
mod page_update;
mod search_engine;
mod tab_name;

pub use browser_nav::resolve_address;
pub use browser_url::BrowserUrl;
pub use error::Error;
pub use page_state::PageState;
pub use page_update::PageUpdate;
pub use search_engine::SearchEngine;
pub use tab_name::tab_name;

/// The pane type id of the browser pane.
pub const BROWSER_PANE_TYPE: &str = "core.browser";
