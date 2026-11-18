//! Navigation logic of the browser pane (`core.browser`).
//!
//! A browser pane loads arbitrary web pages, so it is untrusted by construction: it gets no
//! relay token, no `crowbar://` scheme and no pane protocol (the shell enforces that when it
//! picks the web view kind). This crate holds what the native chrome needs and nothing a page
//! can influence beyond display: [`resolve_address`] turns typed text into a URL,
//! [`PageUrl`] is the only URL form a browser pane loads, [`PageUpdate`] is the engine's
//! report of what happened in the page and [`PageState`] folds those reports into what the
//! address bar, title and reload button show.

mod address;
mod browser;
mod error;
mod script;
mod selection;
mod state;

pub use address::PageUrl;
pub use address::SearchEngine;
pub use browser::{BROWSER_PANE_TYPE, resolve_address};
pub use error::Error;
pub use script::{agent_script, eval_script};
pub use script::{cancel_pick_script, pick_script};
pub use selection::PickReport;
pub use selection::PointerItem;
pub use selection::{pick_count, pointer_text};
pub use state::AgentAnswer;
pub use state::PageState;
pub use state::PageUpdate;
pub use state::tab_name;
