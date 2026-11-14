//! The Hosts page.

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::heading::Heading;
use crate::list_block::ListBlock;
use crate::page::Page;

pub(super) const HOSTS: &str = "hosts";

pub(super) fn page() -> Page {
    Page::new("hosts", "Hosts", "hard-drives")
        .block(Heading::new(
            "Hosts",
            "Crowbar always runs on this computer. Add another machine running its own daemon to reach it from here too.",
        ))
        .block(ListBlock::new(HOSTS, "No hosts.").states("Loading hosts...", "Crowbar could not load the hosts."))
        .block(ButtonSpec::new("Add host", Action::backend("hosts.add")).icon("plus"))
}

#[cfg(test)]
#[path = "hosts_tests.rs"]
mod tests;
