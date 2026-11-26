//! One page of the settings rail.

use crate::keys::ShortcutGroup;
use crate::schema::block::Block;
use crate::schema::page_id::PageId;
use crate::schema::row::Row;
use crate::schema::search_entry::SearchEntry;

/// A rail entry and the blocks it shows.
#[derive(Debug, Clone, PartialEq)]
pub struct Page {
    pub(crate) id: PageId,
    pub(crate) label: String,
    pub(crate) icon: String,
    pub(crate) blocks: Vec<Block>,
    pub(crate) index: Vec<SearchEntry>,
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

    pub(crate) fn new(
        id: impl Into<PageId>,
        label: impl Into<String>,
        icon: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            icon: icon.into(),
            blocks: Vec::new(),
            index: Vec::new(),
        }
    }

    #[must_use]
    pub(crate) fn block(
        mut self,
        block: impl Into<Block>,
    ) -> Self {
        self.blocks.push(block.into());
        self
    }

    #[must_use]
    pub(crate) fn indexed(
        mut self,
        entries: impl IntoIterator<Item = SearchEntry>,
    ) -> Self {
        self.index.extend(entries);
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
