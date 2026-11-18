//! Reading the values on screen: switches, field text, drafts and what can be reset.

use crate::schema::Control;
use crate::schema::Issue;
use crate::schema::Row;
use crate::state::State;
use crate::values::Value;

impl State {
    /// Whether the switch of `row` shows as on.
    pub fn switch_on(
        &self,
        row: &Row,
    ) -> bool {
        let stored = self
            .document
            .value(&row.key)
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let inverted = matches!(row.control, Control::Switch { invert: true });
        stored != inverted
    }

    /// The text of the field `row`: the draft being typed, or the stored value.
    pub fn field_text(
        &self,
        row: &Row,
    ) -> String {
        self.document.text(row)
    }

    /// The problem with what is typed in `row`, if any.
    pub fn issue(
        &self,
        row: &Row,
    ) -> Option<Issue> {
        let text = self.document.text(row);
        row.control.issue(&text)
    }

    /// Whether the field of `row` holds typing that is not stored yet.
    pub fn is_dirty(
        &self,
        row: &Row,
    ) -> bool {
        self.document.is_dirty(row)
    }

    /// Whether a write is still waiting for the backend.
    pub fn has_unsaved(&self) -> bool {
        self.document.has_unsaved()
    }

    /// Whether `row` shows its reset icon: it has a default and holds something else.
    pub fn resettable(
        &self,
        row: &Row,
    ) -> bool {
        row.default.is_some() && !self.document.is_default(row)
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::Issue;
    use crate::state::test_support::{custom_row, gitignore_row, state, state_with, write};
    use crate::values::Value;
    use crate::values::Values;

    #[test]
    fn switches_honour_inversion() {
        let mut state = state();
        let row = gitignore_row();
        assert!(!state.switch_on(&row), "stored true reads as off");
        assert_eq!(
            state.toggle(&row),
            Some(write("showIgnored", Value::Bool(false)))
        );
        assert!(state.switch_on(&row), "stored false reads as on");
        assert!(state.resettable(&row));
        assert_eq!(
            state.reset(&row),
            Some(write("showIgnored", Value::Bool(true)))
        );
        assert!(!state.resettable(&row));
    }

    #[test]
    fn text_is_kept_as_typed_but_flagged_and_describes_itself() {
        let mut state = state_with(
            Values::default().with("engine", "custom"),
            Some("browser.clearData"),
        );
        let row = custom_row();
        assert_eq!(state.description(&row), Some("Search address"));
        state.edit(&row, "not a url");
        assert_eq!(state.issue(&row), Some(Issue::BadSearchTemplate));
        assert_eq!(
            state.description(&row),
            Some("Must be an http(s) address containing %s")
        );
        assert_eq!(state.commit(&row), Some(write("customUrl", "not a url")));
        state.revert(&row);
    }
}
