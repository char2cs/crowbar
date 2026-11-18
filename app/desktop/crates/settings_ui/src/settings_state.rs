//! The pure state machine of the settings modal.

use std::collections::{BTreeMap, BTreeSet};

use crate::choice::Choice;
use crate::chord_capture::ChordCapture;
use crate::document::Document;
use crate::navigation::Navigation;
use crate::page_id::PageId;
use crate::prompt::Prompt;
use crate::schema::Schema;
use crate::scope::Scope;
use crate::snapshot::Snapshot;
use crate::visibility::Visibility;
use chord::Primary;

mod buttons;
mod editing;
mod navigation;
mod outcome;
mod shortcuts;
mod values;
mod visibility;

/// Everything the modal remembers: the schema, the values on screen, the selected page, the search,
/// the field being edited, the shortcut being captured and the confirmation being asked.
///
/// Methods that change a setting return an [`Effect`](crate::effect::Effect); the view performs it against the backend and
/// reports the outcome with [`SettingsState::acknowledged`] or [`SettingsState::write_failed`].
#[derive(Debug, Clone)]
pub struct SettingsState {
    schema: Schema,
    document: Document,
    capabilities: BTreeSet<String>,
    choices: BTreeMap<String, Vec<Choice>>,
    navigation: Navigation,
    query: String,
    visibility: Visibility,
    capture: ChordCapture,
    prompt: Option<Prompt>,
    primary: Primary,
}

impl SettingsState {
    /// A state over `schema` and `snapshot`, opened on `page` (the first visible page when `None` or unknown).
    pub fn new(
        schema: Schema,
        snapshot: Snapshot,
        page: Option<&PageId>,
        primary: Primary,
    ) -> Self {
        let document = Document::new(snapshot.values, schema.defaults().clone());
        let mut state = Self {
            document,
            capabilities: snapshot.capabilities,
            choices: snapshot.choices,
            navigation: Navigation::new(page.cloned().unwrap_or_else(|| PageId::new(""))),
            query: String::new(),
            visibility: Visibility::default(),
            capture: ChordCapture::default(),
            prompt: None,
            primary,
            schema,
        };
        state.refresh();
        state
    }

    fn scope(&self) -> Scope<'_> {
        Scope {
            values: self.document.values(),
            defaults: self.document.defaults(),
            capabilities: &self.capabilities,
        }
    }

    fn refresh(&mut self) {
        self.visibility = Visibility::compute(&self.schema, &self.scope(), &self.query);
        let visible: Vec<&PageId> = self.visibility.pages().iter().collect();
        self.navigation.reconcile(&visible);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::page::Page;
    use crate::page_id::PageId;
    use crate::snapshot::Snapshot;
    use crate::test_support::{schema, size_row, state};
    use chord::Primary;

    #[test]
    fn opens_on_the_requested_page_or_the_first() {
        assert_eq!(state().page().map(Page::label), Some("Appearance"));
        let deep = SettingsState::new(
            schema(),
            Snapshot::default(),
            Some(&PageId::from("browser")),
            Primary::Control,
        );
        assert_eq!(deep.page().map(Page::label), Some("Browser"));
        let unknown = SettingsState::new(
            schema(),
            Snapshot::default(),
            Some(&PageId::from("nope")),
            Primary::Control,
        );
        assert_eq!(
            unknown.page().map(Page::label),
            Some("Appearance"),
            "an unknown deep link falls back to the first page"
        );
    }

    #[test]
    fn revert_drops_the_draft() {
        let mut state = state();
        let row = size_row();
        state.edit(&row, "12");
        state.revert(&row);
        assert!(!state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "15");
    }
}
