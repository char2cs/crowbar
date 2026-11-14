//! One page of the settings rail.

use crate::block::Block;
use crate::condition::Condition;
use crate::page_id::PageId;
use crate::row::Row;
use crate::shortcut_group::ShortcutGroup;

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
    /// A page `id` labelled `label` with the Phosphor icon whose file stem is `icon`.
    pub fn new(id: impl Into<PageId>, label: impl Into<String>, icon: impl Into<String>) -> Self {
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
    pub fn when(mut self, condition: Condition) -> Self {
        self.when = Some(condition);
        self
    }

    /// Appends a block.
    #[must_use]
    pub fn block(mut self, block: impl Into<Block>) -> Self {
        self.blocks.push(block.into());
        self
    }

    pub(crate) fn rows(&self) -> impl Iterator<Item = &Row> {
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
#[path = "page_tests.rs"]
mod tests;
