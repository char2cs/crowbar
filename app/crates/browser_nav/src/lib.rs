//! Navigation logic of the browser pane (`core.browser`).
//!
//! A browser pane loads arbitrary web pages, so it is untrusted by construction: it gets no
//! relay token, no `crowbar://` scheme and no pane protocol (the shell enforces that when it
//! picks the web view kind). This crate holds what the native chrome needs and nothing a page
//! can influence beyond display: [`resolve_address`] turns typed text into a URL,
//! [`BrowserUrl`] is the only URL form a browser pane loads, [`PageUpdate`] is the engine's
//! report of what happened in the page and [`PageState`] folds those reports into what the
//! address bar, title and reload button show.

mod agent_answer;
mod agent_script;
mod browser_nav;
mod browser_url;
mod error;
mod page_calls;
mod page_state;
mod page_update;
mod pick_report;
mod pick_script;
mod pointer_item;
mod pointer_text;
mod search_engine;
mod tab_name;

pub use agent_answer::AgentAnswer;
pub use agent_script::{agent_script, eval_script};
pub use browser_nav::resolve_address;
pub use browser_url::BrowserUrl;
pub use error::Error;
pub use page_state::PageState;
pub use page_update::PageUpdate;
pub use pick_report::PickReport;
pub use pick_script::{cancel_pick_script, pick_script};
pub use pointer_item::PointerItem;
pub use pointer_text::{pick_count, pointer_text};
pub use search_engine::SearchEngine;
pub use tab_name::tab_name;

/// The pane type id of the browser pane.
pub const BROWSER_PANE_TYPE: &str = "core.browser";
