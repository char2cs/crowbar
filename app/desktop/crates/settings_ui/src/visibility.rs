//! Which pages, rows and blocks a search query leaves visible.

use std::collections::BTreeSet;

use crate::block::Block;
use crate::condition::Condition;
use crate::filter::{Filter, matches};
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

fn row_texts(
    row: &Row,
    section: &Section,
) -> String {
    let mut texts = vec![row.label.as_str(), section.title.as_str()];
    texts.extend(row.description.as_deref());
    texts.extend(row.keywords.iter().map(String::as_str));
    texts.join("\n")
}

impl Visibility {
    pub(crate) fn compute(
        schema: &Schema,
        scope: &Scope<'_>,
        query: &str,
    ) -> Self {
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

    fn page_filter<'a>(
        &self,
        page: &Page,
        words: &'a [String],
    ) -> Filter<'a> {
        if !self.searching || matches(words, &[&page.label]) {
            return Filter::Everything;
        }
        Filter::Words(words)
    }

    fn add_page(
        &mut self,
        page: &Page,
        scope: &Scope<'_>,
        words: &[String],
    ) {
        let filter = self.page_filter(page, words);
        let mut any = matches!(filter, Filter::Everything);
        for (index, block) in page.blocks.iter().enumerate() {
            any |= self.add_block((page, index, block), scope, filter);
        }
        if any {
            self.pages.push(page.id.clone());
        }
    }

    fn add_block(
        &mut self,
        (page, index, block): (&Page, usize, &Block),
        scope: &Scope<'_>,
        filter: Filter<'_>,
    ) -> bool {
        match block {
            Block::Section(section) => self.add_rows(section, scope, filter),
            Block::Shortcuts(group) => self.add_shortcuts(group, filter),
            other => self.add_plain((page, index, other), filter),
        }
    }

    fn add_plain(
        &mut self,
        (page, index, block): (&Page, usize, &Block),
        filter: Filter<'_>,
    ) -> bool {
        let shown = filter.admits(&[&block_text(block)]);
        if shown {
            self.blocks.insert((page.id.clone(), index));
        }
        shown
    }

    fn add_rows(
        &mut self,
        section: &Section,
        scope: &Scope<'_>,
        filter: Filter<'_>,
    ) -> bool {
        let shown: Vec<&Row> = section
            .rows
            .iter()
            .filter(|row| holds(row.when.as_ref(), scope))
            .filter(|row| filter.admits(&[&row_texts(row, section)]))
            .collect();
        self.rows.extend(shown.iter().map(|row| row.key.clone()));
        !shown.is_empty()
    }

    fn add_shortcuts(
        &mut self,
        group: &ShortcutGroup,
        filter: Filter<'_>,
    ) -> bool {
        let shown: Vec<&Shortcut> = group
            .shortcuts
            .iter()
            .filter(|shortcut| {
                let texts = [
                    shortcut.label.as_str(),
                    shortcut.command.as_str(),
                    group.title.as_str(),
                ];
                filter.admits(&texts)
            })
            .collect();
        self.shortcuts
            .extend(shown.iter().map(|shortcut| shortcut.command.clone()));
        !shown.is_empty()
    }

    pub(crate) fn pages(&self) -> &[PageId] {
        &self.pages
    }

    pub(crate) fn row(
        &self,
        row: &Row,
    ) -> bool {
        self.rows.contains(&row.key)
    }

    pub(crate) fn shortcut(
        &self,
        shortcut: &Shortcut,
    ) -> bool {
        self.shortcuts.contains(&shortcut.command)
    }

    pub(crate) fn block(
        &self,
        page: &PageId,
        index: usize,
    ) -> bool {
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

fn holds(
    rule: Option<&Condition>,
    scope: &Scope<'_>,
) -> bool {
    rule.is_none_or(|rule| rule.holds(scope))
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;

    use super::*;
    use crate::condition::Condition;
    use crate::control::Control;
    use crate::heading::Heading;
    use crate::values::Values;

    fn schema() -> Schema {
        let appearance = Page::new("appearance", "Appearance", "paint-brush").block(
            Section::new("Typography")
                .row(
                    Row::new("uiFontSize", "UI Font Size", Control::switch())
                        .describe("Adjust UI text")
                        .keywords(&["zoom"]),
                )
                .row(Row::new(
                    "sidebarPosition",
                    "Sidebar Position",
                    Control::switch(),
                )),
        );
        let browser = Page::new("browser", "Browser", "globe").block(
            Section::new("Navigation")
                .row(Row::new("browserHomepage", "Homepage", Control::switch()))
                .row(
                    Row::new("customUrl", "Custom Search URL", Control::switch())
                        .when(Condition::equals("engine", "custom")),
                ),
        );
        let keys = Page::new("keybindings", "Keybindings", "keyboard").block(
            ShortcutGroup::new("Tabs", "overrides")
                .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t")),
        );
        let hosts = Page::new("hosts", "Hosts", "hard-drives")
            .block(Heading::new("Hosts", "Add another machine."));
        let hidden = Page::new("dev", "Developer", "bug").when(Condition::capability("dev"));
        Schema::new(vec![appearance, browser, keys, hosts, hidden]).unwrap()
    }

    fn compute(
        schema: &Schema,
        query: &str,
    ) -> Visibility {
        let values = Values::default();
        let capabilities = BTreeSet::new();
        let scope = Scope {
            values: &values,
            defaults: schema.defaults(),
            capabilities: &capabilities,
        };
        Visibility::compute(schema, &scope, query)
    }

    fn page_ids(visibility: &Visibility) -> Vec<&str> {
        visibility.pages().iter().map(PageId::as_str).collect()
    }

    #[test]
    fn without_a_query_every_applicable_page_and_row_shows() {
        let schema = schema();
        let visibility = compute(&schema, "  ");
        assert_eq!(
            page_ids(&visibility),
            ["appearance", "browser", "keybindings", "hosts"]
        );
        let shown: Vec<&str> = schema
            .rows()
            .filter(|row| visibility.row(row))
            .map(|row| row.key.as_str())
            .collect();
        assert_eq!(
            shown,
            ["uiFontSize", "sidebarPosition", "browserHomepage"],
            "the conditional row is hidden"
        );
    }

    #[test]
    fn one_matcher_decides_pages_and_rows_together() {
        type Case = (
            &'static str,
            &'static [&'static str],
            &'static [&'static str],
        );
        const CASES: &[Case] = &[
            ("font", &["appearance"], &["uiFontSize"]),
            ("zoom", &["appearance"], &["uiFontSize"]),
            ("adjust text", &["appearance"], &["uiFontSize"]),
            (
                "typography",
                &["appearance"],
                &["uiFontSize", "sidebarPosition"],
            ),
            ("browser", &["browser"], &["browserHomepage"]),
            ("zzz", &[], &[]),
        ];
        let schema = schema();
        for &(query, pages, rows) in CASES {
            let visibility = compute(&schema, query);
            assert_eq!(page_ids(&visibility), pages, "pages for {query:?}");
            let shown: Vec<&str> = schema
                .rows()
                .filter(|row| visibility.row(row))
                .map(|row| row.key.as_str())
                .collect();
            assert_eq!(shown, rows, "rows for {query:?}");
        }
    }

    #[test]
    fn shortcuts_and_heading_pages_are_searchable_too() {
        let schema = schema();
        let by_label = compute(&schema, "new tab");
        assert_eq!(page_ids(&by_label), ["keybindings"]);
        assert!(
            schema
                .shortcuts()
                .all(|(_, shortcut)| by_label.shortcut(shortcut))
        );
        let by_text = compute(&schema, "another machine");
        assert_eq!(page_ids(&by_text), ["hosts"]);
        assert!(by_text.block(&PageId::from("hosts"), 0));
        let by_page = compute(&schema, "hosts");
        assert!(
            by_page.block(&PageId::from("hosts"), 0),
            "a matching page label shows all its blocks"
        );
    }
}
