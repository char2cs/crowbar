use super::*;
use crate::block::Block;

#[test]
fn a_heading_and_one_list_without_rows() {
    let page = page();
    assert_eq!(page.blocks.len(), 2);
    assert_eq!(page.rows().count(), 0);
    assert!(matches!(page.blocks.last(), Some(Block::List(list)) if list.source == SESSIONS));
}
