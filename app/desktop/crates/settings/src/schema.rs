//! The data the modal renders: pages of blocks.

mod block;
mod button_spec;
mod condition;
mod control;
mod page;
mod page_id;
pub(crate) mod row;
mod scope;
mod search_entry;

pub use block::{Block, Heading, ListBlock, Section};
pub use button_spec::{Action, ButtonFace, ButtonSpec, Confirm};
pub(crate) use condition::Condition;
pub(crate) use control::OnLoad;
pub use control::{ChoiceSource, Control, ControlWidth, Issue, Nudge, NumberRange, Validator};
pub use page::Page;
pub use page_id::PageId;
pub use row::{LabelStyle, Row};
pub(crate) use scope::Scope;
pub(crate) use search_entry::SearchEntry;

use std::collections::BTreeSet;

use crate::keys::Shortcut;
use crate::keys::ShortcutGroup;
use crate::values::Error;
use crate::values::Key;
use crate::values::Values;

/// Every page of the modal, in rail order, with the defaults the rows declare.
#[derive(Debug, Clone, PartialEq)]
pub struct Schema {
    pages: Vec<Page>,
    defaults: Values,
}

impl Schema {
    pub(crate) fn new(pages: Vec<Page>) -> Result<Self, Error> {
        let mut schema = Self {
            pages: Vec::new(),
            defaults: Values::default(),
        };
        let (mut ids, mut keys) = (BTreeSet::new(), BTreeSet::new());
        pages
            .iter()
            .try_for_each(|page| schema.claim(page, (&mut ids, &mut keys)))?;
        schema.pages = pages;
        Ok(schema)
    }

    fn claim(
        &mut self,
        page: &Page,
        (ids, keys): (&mut BTreeSet<PageId>, &mut BTreeSet<Key>),
    ) -> Result<(), Error> {
        if !ids.insert(page.id.clone()) {
            return Err(duplicate("page", page.id.as_str()));
        }
        page.rows().try_for_each(|row| self.claim_row(row, keys))
    }

    fn claim_row(
        &mut self,
        row: &Row,
        keys: &mut BTreeSet<Key>,
    ) -> Result<(), Error> {
        if !keys.insert(row.key.clone()) {
            return Err(duplicate("key", row.key.as_str()));
        }
        if let Some(default) = &row.default {
            self.defaults.set(row.key.clone(), default.clone());
        }
        Ok(())
    }

    /// The pages in rail order.
    pub fn pages(&self) -> &[Page] {
        &self.pages
    }

    pub(crate) fn page(
        &self,
        id: &PageId,
    ) -> Option<&Page> {
        self.pages.iter().find(|page| &page.id == id)
    }

    pub(crate) fn defaults(&self) -> &Values {
        &self.defaults
    }

    /// The rows of every page, in rail order.
    pub fn rows(&self) -> impl Iterator<Item = &Row> {
        self.pages.iter().flat_map(Page::rows)
    }

    pub(crate) fn shortcuts(&self) -> impl Iterator<Item = (&ShortcutGroup, &Shortcut)> {
        self.pages
            .iter()
            .flat_map(Page::shortcut_groups)
            .flat_map(|group| {
                group
                    .shortcuts
                    .iter()
                    .map(move |shortcut| (group, shortcut))
            })
    }
}

fn duplicate(
    what: &'static str,
    name: &str,
) -> Error {
    Error::Duplicate {
        what,
        name: name.to_owned(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::block::Section;
    use crate::schema::control::Control;
    use crate::values::Value;

    fn page(
        id: &str,
        key: &str,
    ) -> Page {
        Page::new(id, id, "gear").block(
            Section::new("S")
                .row(Row::new(key, key, Control::switch()).default_value(Value::Bool(true))),
        )
    }

    #[test]
    fn defaults_come_from_the_rows() {
        let schema = Schema::new(vec![page("a", "x"), page("b", "y")]).unwrap();
        assert_eq!(
            schema.defaults().get(&Key::from("y")),
            Some(&Value::Bool(true))
        );
    }

    #[test]
    fn repeated_pages_and_keys_are_rejected() {
        let same_page = Schema::new(vec![page("a", "x"), page("a", "y")]);
        assert!(
            matches!(same_page, Err(Error::Duplicate { what: "page", .. })),
            "{same_page:?}"
        );
        let same_key = Schema::new(vec![page("a", "x"), page("b", "x")]);
        assert!(
            matches!(same_key, Err(Error::Duplicate { what: "key", .. })),
            "{same_key:?}"
        );
    }
}
