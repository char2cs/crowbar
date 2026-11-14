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
