//! One page of the settings rail.

use crate::keys::ShortcutGroup;
use crate::schema::block::Block;
use crate::schema::condition::Condition;
use crate::schema::page_id::PageId;
use crate::schema::row::Row;

/// A rail entry and the blocks it shows.
#[derive(Debug, Clone, PartialEq)]
pub struct Page {
    pub(crate) id: PageId,
    pub(crate) label: String,
    pub(crate) icon: String,
    pub(crate) when: Option<Condition>,
    pub(crate) blocks: Vec<Block>,
}

impl Page {
    /// File stem of the Phosphor icon shown in the rail.
    #[must_use]
    pub fn icon(&self) -> &str {
        &self.icon
    }

    /// The blocks the page shows, in order.
    #[must_use]
    pub fn blocks(&self) -> &[Block] {
        &self.blocks
    }

    /// A page `id` labelled `label` with the Phosphor icon whose file stem is `icon`.
    pub fn new(
        id: impl Into<PageId>,
        label: impl Into<String>,
        icon: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            icon: icon.into(),
            when: None,
            blocks: Vec::new(),
        }
    }

    /// Shows the page only while `condition` holds.
    #[must_use]
    pub fn when(
        mut self,
        condition: Condition,
    ) -> Self {
        self.when = Some(condition);
        self
    }

    /// Appends a block.
    #[must_use]
    pub fn block(
        mut self,
        block: impl Into<Block>,
    ) -> Self {
        self.blocks.push(block.into());
        self
    }

    /// The rows of every section on the page, in order.
    pub fn rows(&self) -> impl Iterator<Item = &Row> {
        self.blocks.iter().flat_map(|block| match block {
            Block::Section(section) => section.rows.as_slice(),
            _ => &[],
        })
    }

    pub(crate) fn shortcut_groups(&self) -> impl Iterator<Item = &ShortcutGroup> {
        self.blocks.iter().filter_map(|block| match block {
            Block::Shortcuts(group) => Some(group),
            _ => None,
        })
    }

    /// The page id.
    pub fn id(&self) -> &PageId {
        &self.id
    }

    /// The rail label.
    pub fn label(&self) -> &str {
        &self.label
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::block::Section;

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
        use crate::keys::ShortcutGroup;
        use crate::schema::control::Control;
        use crate::schema::row::Row;
        let page = Page::new("p", "P", "gear")
            .block(Section::new("A").row(Row::new("a", "A", Control::switch())))
            .block(ShortcutGroup::new("G", "overrides"))
            .block(Section::new("B").row(Row::new("b", "B", Control::switch())));
        let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
        assert_eq!(keys, ["a", "b"]);
        assert_eq!(page.shortcut_groups().count(), 1);
    }
}
