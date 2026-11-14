//! Which pages, rows and blocks a search query leaves visible.

use std::collections::BTreeSet;

use crate::block::Block;
use crate::condition::Condition;
use crate::key::Key;
use crate::page::Page;
use crate::page_id::PageId;
use crate::row::Row;
use crate::schema::Schema;
use crate::scope::Scope;
use crate::section::Section;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;

#[derive(Debug, Clone, Default)]
pub(crate) struct Visibility {
    searching: bool,
    pages: Vec<PageId>,
    rows: BTreeSet<Key>,
    shortcuts: BTreeSet<String>,
    blocks: BTreeSet<(PageId, usize)>,
}

fn words(query: &str) -> Vec<String> {
    query.split_whitespace().map(str::to_lowercase).collect()
}

fn matches(words: &[String], texts: &[&str]) -> bool {
    let haystack = texts.join("\n").to_lowercase();
    words.iter().all(|word| haystack.contains(word.as_str()))
}

fn row_texts(row: &Row, section: &Section) -> String {
    let mut texts = vec![row.label.as_str(), section.title.as_str()];
    texts.extend(row.description.as_deref());
    texts.extend(row.keywords.iter().map(String::as_str));
    texts.join("\n")
}

impl Visibility {
    pub(crate) fn compute(schema: &Schema, scope: &Scope<'_>, query: &str) -> Self {
        let words = words(query);
        let mut visibility = Self {
            searching: !words.is_empty(),
            ..Self::default()
        };
        for page in schema
            .pages()
            .iter()
            .filter(|page| holds(page.when.as_ref(), scope))
        {
            visibility.add_page(page, scope, &words);
        }
        visibility
    }

    fn add_page(&mut self, page: &Page, scope: &Scope<'_>, words: &[String]) {
        let whole = !self.searching || matches(words, &[&page.label]);
        let mut any = whole;
        for (index, block) in page.blocks.iter().enumerate() {
            any |= self.add_block((page, index, block), scope, (words, whole));
        }
        if any {
            self.pages.push(page.id.clone());
        }
    }

    fn add_block(
        &mut self,
        (page, index, block): (&Page, usize, &Block),
        scope: &Scope<'_>,
        (words, whole): (&[String], bool),
    ) -> bool {
        match block {
            Block::Section(section) => self.add_rows(section, scope, (words, whole)),
            Block::Shortcuts(group) => self.add_shortcuts(group, (words, whole)),
            other => self.add_plain((page, index, other), (words, whole)),
        }
    }

    fn add_plain(
        &mut self,
        (page, index, block): (&Page, usize, &Block),
        (words, whole): (&[String], bool),
    ) -> bool {
        let shown = whole || matches(words, &[&block_text(block)]);
        if shown {
            self.blocks.insert((page.id.clone(), index));
        }
        shown
    }

    fn add_rows(
        &mut self,
        section: &Section,
        scope: &Scope<'_>,
        (words, whole): (&[String], bool),
    ) -> bool {
        let shown: Vec<&Row> = section
            .rows
            .iter()
            .filter(|row| holds(row.when.as_ref(), scope))
            .filter(|row| whole || matches(words, &[&row_texts(row, section)]))
            .collect();
        self.rows.extend(shown.iter().map(|row| row.key.clone()));
        !shown.is_empty()
    }

    fn add_shortcuts(&mut self, group: &ShortcutGroup, (words, whole): (&[String], bool)) -> bool {
        let shown: Vec<&Shortcut> = group
            .shortcuts
            .iter()
            .filter(|shortcut| {
                let texts = [
                    shortcut.label.as_str(),
                    shortcut.command.as_str(),
                    group.title.as_str(),
                ];
                whole || matches(words, &texts)
            })
            .collect();
        self.shortcuts
            .extend(shown.iter().map(|shortcut| shortcut.command.clone()));
        !shown.is_empty()
    }

    pub(crate) fn pages(&self) -> &[PageId] {
        &self.pages
    }

    pub(crate) fn row(&self, row: &Row) -> bool {
        self.rows.contains(&row.key)
    }

    pub(crate) fn shortcut(&self, shortcut: &Shortcut) -> bool {
        self.shortcuts.contains(&shortcut.command)
    }

    pub(crate) fn block(&self, page: &PageId, index: usize) -> bool {
        self.blocks.contains(&(page.clone(), index))
    }
}

fn block_text(block: &Block) -> String {
    match block {
        Block::Heading(heading) => format!("{}\n{}", heading.title, heading.description),
        Block::List(list) => format!(
            "{}\n{}",
            list.title.as_deref().unwrap_or_default(),
            list.intro.as_deref().unwrap_or_default()
        ),
        Block::Button(button) => button.label.clone(),
        Block::Section(_) | Block::Shortcuts(_) => String::new(),
    }
}

fn holds(rule: Option<&Condition>, scope: &Scope<'_>) -> bool {
    rule.is_none_or(|rule| rule.holds(scope))
}

#[cfg(test)]
#[path = "visibility_tests.rs"]
mod tests;
