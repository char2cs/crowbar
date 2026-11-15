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
mod tests {
    use super::*;
    use crate::block::Block;

    #[test]
    fn heading_list_and_add_button_in_order() {
        let page = page();
        let kinds: Vec<&str> = page
            .blocks
            .iter()
            .map(|block| match block {
                Block::Heading(_) => "heading",
                Block::List(_) => "list",
                Block::Button(_) => "button",
                _ => "other",
            })
            .collect();
        assert_eq!(kinds, ["heading", "list", "button"]);
    }
}
