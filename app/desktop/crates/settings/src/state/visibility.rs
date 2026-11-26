//! Which pages, rows and blocks a search query leaves visible.
//! Spec: web/src/features/settings/components/settings-section.tsx (row and section filtering)
//! and settings-vertical-tabs.tsx (pages by index hit). A block outside any section goes with the
//! section above it (else the first below), as develop draws it inside that section.

use std::collections::BTreeSet;

use crate::schema::Block;
use crate::schema::Condition;
use crate::schema::Page;
use crate::schema::PageId;
use crate::schema::Row;
use crate::schema::Schema;
use crate::schema::Scope;
use crate::schema::Section;
use crate::state::visibility::query::Query;
use crate::state::visibility::slot::Slot;
use crate::values::Key;

mod conditions;
mod query;
mod slot;

#[derive(Debug, Clone, Default)]
pub(crate) struct Visibility {
    pages: Vec<PageId>,
    rows: BTreeSet<Key>,
    blocks: BTreeSet<(PageId, usize)>,
}

impl Visibility {
    pub(crate) fn compute(
        schema: &Schema,
        scope: &Scope<'_>,
        query: &str,
    ) -> Self {
        let query = Query::new(query);
        let mut visibility = Self::default();
        for page in schema.pages().iter().filter(|page| {
            query.is_blank() || page.index.iter().any(|entry| query.finds_entry(entry))
        }) {
            visibility.add_page(page, scope, &query);
        }
        visibility
    }

    fn add_page(
        &mut self,
        page: &Page,
        scope: &Scope<'_>,
        query: &Query,
    ) {
        self.pages.push(page.id.clone());
        let slots: Vec<Slot> = page
            .blocks
            .iter()
            .map(|block| self.slot(block, scope, query))
            .collect();
        let shown = (0..slots.len())
            .filter(|&index| slots.get(index) == Some(&Slot::Plain) && owner_shown(&slots, index))
            .map(|index| (page.id.clone(), index));
        self.blocks.extend(shown);
    }

    fn slot(
        &mut self,
        block: &Block,
        scope: &Scope<'_>,
        query: &Query,
    ) -> Slot {
        let Block::Section(section) = block else {
            return Slot::Plain;
        };
        if self.add_rows(section, scope, query) {
            return Slot::Shown;
        }
        Slot::Hidden
    }

    fn add_rows(
        &mut self,
        section: &Section,
        scope: &Scope<'_>,
        query: &Query,
    ) -> bool {
        let shown: Vec<&Row> = section
            .rows
            .iter()
            .filter(|row| holds(row.when.as_ref(), scope))
            .filter(|row| query.finds_row(row))
            .collect();
        self.rows.extend(shown.iter().map(|row| row.key.clone()));
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

    pub(crate) fn block(
        &self,
        page: &PageId,
        index: usize,
    ) -> bool {
        self.blocks.contains(&(page.clone(), index))
    }
}

fn owner_shown(
    slots: &[Slot],
    index: usize,
) -> bool {
    let (before, after) = slots.split_at(index.min(slots.len()));
    let above = before.iter().rev().find_map(|slot| slot.section());
    let below = || after.iter().find_map(|slot| slot.section());
    above.or_else(below).is_none_or(|slot| slot == Slot::Shown)
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
    use crate::keys::Shortcut;
    use crate::keys::ShortcutGroup;
    use crate::schema::Control;
    use crate::schema::Heading;
    use crate::schema::SearchEntry;
    use crate::values::Values;

    fn appearance() -> Page {
        Page::new("appearance", "Appearance", "paint-brush")
            .indexed([
                SearchEntry::new("Typography", "UI Font Size", "Adjust UI text")
                    .keywords(&["zoom"]),
            ])
            .block(
                Section::new("Typography")
                    .row(
                        Row::new("uiFontSize", "UI Font Size", Control::switch())
                            .describe("Adjust UI text"),
                    )
                    .row(Row::new(
                        "sidebarPosition",
                        "Sidebar Position",
                        Control::switch(),
                    )),
            )
    }

    fn agents() -> Page {
        Page::new("providers", "Agents", "robot")
            .indexed([SearchEntry::new(
                "Chat",
                "Chat By Default",
                "Open a chat on Chat rather than Terminal",
            )])
            .block(Heading::new("Agents", "The coding CLIs."))
            .block(Section::new("Agents").row(Row::new(
                "permission",
                "Default permission level",
                Control::switch(),
            )))
            .block(Heading::new("Agents list", "Drag to reorder."))
            .block(
                Section::new("Chat").row(
                    Row::new("chatDefault", "Chat By Default", Control::switch())
                        .describe("Open a chat on Chat rather than Terminal")
                        .when(Condition::capability("chat")),
                ),
            )
    }

    fn schema() -> Schema {
        let (appearance, agents) = (appearance(), agents());
        let keys = Page::new("keybindings", "Keybindings", "keyboard").block(
            ShortcutGroup::new("Tabs", "overrides")
                .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t")),
        );
        Schema::new(vec![appearance, agents, keys]).unwrap()
    }

    fn compute(
        schema: &Schema,
        query: &str,
        capability: Option<&str>,
    ) -> Visibility {
        let values = Values::default();
        let capabilities: BTreeSet<String> = capability.into_iter().map(str::to_owned).collect();
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

    fn rows<'a>(
        schema: &'a Schema,
        visibility: &Visibility,
    ) -> Vec<&'a str> {
        schema
            .rows()
            .filter(|row| visibility.row(row))
            .map(|row| row.key.as_str())
            .collect()
    }

    #[test]
    fn without_a_query_every_applicable_page_row_and_block_shows() {
        let schema = schema();
        let visibility = compute(&schema, "  ", None);
        assert_eq!(
            page_ids(&visibility),
            ["appearance", "providers", "keybindings"]
        );
        assert_eq!(
            rows(&schema, &visibility),
            ["uiFontSize", "sidebarPosition", "permission"],
            "the conditional row is hidden"
        );
        assert!(visibility.block(&PageId::from("keybindings"), 0));
    }

    #[test]
    fn the_index_finds_pages_and_the_row_text_finds_rows() {
        type Case = (
            &'static str,
            &'static [&'static str],
            &'static [&'static str],
        );
        const CASES: &[Case] = &[
            ("zoom", &["appearance"], &[]),
            ("font size", &["appearance"], &["uiFontSize"]),
            ("adjust ui", &["appearance"], &["uiFontSize"]),
            ("typography", &["appearance"], &[]),
            ("terminal", &["providers"], &["chatDefault"]),
            ("new tab", &[], &[]),
            ("zzz", &[], &[]),
        ];
        let schema = schema();
        for &(query, pages, shown) in CASES {
            let visibility = compute(&schema, query, Some("chat"));
            assert_eq!(page_ids(&visibility), pages, "pages for {query:?}");
            assert_eq!(rows(&schema, &visibility), shown, "rows for {query:?}");
        }
    }

    #[test]
    fn a_plain_block_follows_the_section_it_sits_under() {
        let schema = schema();
        let agents = PageId::from("providers");
        let chat = compute(&schema, "terminal", Some("chat"));
        assert!(
            !chat.block(&agents, 0),
            "the heading goes with the hidden Agents section"
        );
        assert!(!chat.block(&agents, 2), "so does the list under it");
        let all = compute(&schema, "", None);
        assert!(all.block(&agents, 0) && all.block(&agents, 2));
    }
}
