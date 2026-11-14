use super::*;
use crate::section::Section;

#[test]
fn blocks_append_in_order() {
    let page = Page::new("git", "Git", "git-branch")
        .when(Condition::capability("git"))
        .block(Section::new("One"))
        .block(Section::new("Two"));
    assert_eq!((page.id().as_str(), page.label()), ("git", "Git"));
    assert_eq!(page.icon, "git-branch");
    assert!(page.when.is_some());
    assert_eq!(page.blocks.len(), 2);
}

#[test]
fn rows_and_shortcut_groups_are_found_across_blocks() {
    use crate::control::Control;
    use crate::row::Row;
    use crate::shortcut_group::ShortcutGroup;
    let page = Page::new("p", "P", "gear")
        .block(Section::new("A").row(Row::new("a", "A", Control::switch())))
        .block(ShortcutGroup::new("G", "overrides"))
        .block(Section::new("B").row(Row::new("b", "B", Control::switch())));
    let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
    assert_eq!(keys, ["a", "b"]);
    assert_eq!(page.shortcut_groups().count(), 1);
}
