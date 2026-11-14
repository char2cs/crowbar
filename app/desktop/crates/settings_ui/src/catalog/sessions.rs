//! The Active sessions page.

use crate::heading::Heading;
use crate::list_block::ListBlock;
use crate::page::Page;

pub(super) const SESSIONS: &str = "sessions";

pub(super) fn page() -> Page {
    Page::new("active-sessions", "Active sessions", "pulse")
        .block(Heading::new(
            "Active sessions",
            "Closing a chat only detaches from it; its agent keeps running. Kill one here to end it.",
        ))
        .block(ListBlock::new(SESSIONS, "No agents are running.").states("Loading sessions...", "Crowbar could not load the sessions."))
}

#[cfg(test)]
#[path = "sessions_tests.rs"]
mod tests;
