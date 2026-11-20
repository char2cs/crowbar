//! The data the modal renders: pages of blocks.

mod block;
mod button_spec;
mod condition;
mod control;
mod page;
mod page_id;
mod row;
mod scope;
mod search_entry;

pub use block::{Block, Heading, ListBlock, Section};
pub use button_spec::{Action, ButtonSpec, Confirm};
pub use condition::Condition;
pub use control::{ChoiceSource, Control, ControlWidth, Issue, Nudge, NumberRange, Validator};
pub use page::Page;
pub use page_id::PageId;
pub use row::Row;
pub(crate) use scope::Scope;
pub use search_entry::SearchEntry;

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
    /// A schema of `pages`.
    ///
    /// # Errors
    /// [`Error::Duplicate`] when a page id or a setting key repeats.
    pub fn new(pages: Vec<Page>) -> Result<Self, Error> {
        let mut schema = Self {
            pages: Vec::new(),
            defaults: Values::default(),
        };
        schema.append(pages)?;
        Ok(schema)
    }

    /// Adds the pages of the extension `extension`; their ids start with `ext.<extension>` and
    /// their keys with `ext.<extension>.`, so an extension cannot touch core settings.
    ///
    /// # Errors
    /// [`Error::OutsideNamespace`] for a page or key outside the namespace, [`Error::Duplicate`]
    /// for a repeated id or key; the schema is unchanged on error.
    pub fn extend(
        &mut self,
        extension: &str,
        pages: Vec<Page>,
    ) -> Result<(), Error> {
        let namespace = format!("ext.{extension}");
        let outside = |name: &str| Error::OutsideNamespace {
            extension: extension.to_owned(),
            name: name.to_owned(),
        };
        if let Some(name) = pages.iter().find_map(|page| stray(page, &namespace)) {
            return Err(outside(name));
        }
        let mut grown = self.clone();
        grown.append(pages)?;
        *self = grown;
        Ok(())
    }

    fn append(
        &mut self,
        pages: Vec<Page>,
    ) -> Result<(), Error> {
        let mut ids: BTreeSet<PageId> = self.pages.iter().map(|page| page.id.clone()).collect();
        let mut keys: BTreeSet<Key> = self.rows().map(|row| row.key.clone()).collect();
        pages
            .iter()
            .try_for_each(|page| self.claim(page, (&mut ids, &mut keys)))?;
        self.pages.extend(pages);
        Ok(())
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

    /// Every shortcut of every page, in rail order, paired with the group it belongs to.
    pub fn shortcuts(&self) -> impl Iterator<Item = (&ShortcutGroup, &Shortcut)> {
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

fn stray<'a>(
    page: &'a Page,
    namespace: &str,
) -> Option<&'a str> {
    let id = page.id.as_str();
    if id != namespace && !Key::new(id).is_in(namespace) {
        return Some(id);
    }
    page.rows()
        .find(|row| !row.key.is_in(namespace))
        .map(|row| row.key.as_str())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::block::Section;
    use crate::schema::control::Control;
    use crate::test_support::{Case, run_cases};
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

    fn defaults_come_from_the_rows() {
        let schema = Schema::new(vec![page("a", "x"), page("b", "y")]).unwrap();
        assert_eq!(
            schema.defaults().get(&Key::from("y")),
            Some(&Value::Bool(true))
        );
        assert_eq!(schema.pages().len(), 2);
        assert!(schema.page(&PageId::from("b")).is_some());
        assert_eq!(schema.rows().count(), 2);
    }

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

    fn extension_pages_stay_in_their_namespace() {
        let mut schema = Schema::new(vec![page("core", "x")]).unwrap();
        schema
            .extend("git", vec![page("ext.git", "ext.git.autoFetch")])
            .unwrap();
        assert_eq!(schema.pages().len(), 2);
        let stealing = schema.extend("git", vec![page("ext.git.more", "x2")]);
        assert!(
            matches!(stealing, Err(Error::OutsideNamespace { .. })),
            "{stealing:?}"
        );
        let other_page = schema.extend("git", vec![page("ext.gitlab", "ext.git.k")]);
        assert!(
            matches!(other_page, Err(Error::OutsideNamespace { .. })),
            "{other_page:?}"
        );
        let repeated = schema.extend("git", vec![page("ext.git", "ext.git.other")]);
        assert!(
            matches!(repeated, Err(Error::Duplicate { .. })),
            "{repeated:?}"
        );
        assert_eq!(
            schema.pages().len(),
            2,
            "a failed extension leaves the schema unchanged"
        );
    }

    const CASES: &[Case] = &[
        ("defaults come from the rows", defaults_come_from_the_rows),
        (
            "repeated pages and keys are rejected",
            repeated_pages_and_keys_are_rejected,
        ),
        (
            "extension pages stay in their namespace",
            extension_pages_stay_in_their_namespace,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
