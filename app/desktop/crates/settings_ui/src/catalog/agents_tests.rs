use super::*;
use crate::block::Block;

#[test]
fn the_page_lists_agents_then_descriptor_checks_then_the_chat_section() {
    let page = page();
    let lists: Vec<&str> = page
        .blocks
        .iter()
        .filter_map(|block| match block {
            Block::List(list) => Some(list.source.as_str()),
            _ => None,
        })
        .collect();
    assert_eq!(lists, [AGENTS, DESCRIPTORS]);
    assert!(matches!(page.blocks.first(), Some(Block::Heading(_))));
    assert!(
        page.rows()
            .any(|row| row.key().as_str() == "defaultPermissionLevel")
    );
}
