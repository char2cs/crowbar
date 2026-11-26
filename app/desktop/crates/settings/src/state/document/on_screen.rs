//! Reading the values on screen: switches, field text, drafts and what can be reset.

use crate::schema::Control;
use crate::schema::Issue;
use crate::schema::Row;
use crate::schema::row::ResetOffer;
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

    /// Whether `row` shows its reset icon: it has a default and holds something else.
    pub fn resettable(
        &self,
        row: &Row,
    ) -> bool {
        row.reset == ResetOffer::Offered && row.default.is_some() && !self.document.is_default(row)
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

    fn stored_as(
        key: &str,
        stored: Option<f64>,
    ) -> Values {
        stored.map_or_else(Values::default, |number| {
            Values::default().with(key, number)
        })
    }

    #[test]
    fn a_stored_number_shows_as_develop_loads_it() {
        let schema = crate::default_schema().unwrap();
        let cases = [
            ("editorLineHeight", None, "1.4"),
            ("editorLineHeight", Some(1.43), "1.4"),
            ("editorLineHeight", Some(14.0), "2"),
            ("uiFontSize", Some(99.0), "24"),
            ("markdownFontSize", Some(30.0), "24"),
            ("fileTreeIndentSize", Some(40.0), "32"),
            ("browserZoom", Some(999.0), "300"),
            ("terminalLineHeight", Some(1.43), "1.4"),
            ("terminalLineHeight", Some(5.0), "5"),
            ("terminalLineHeight", Some(1.2), "1"),
            ("terminalLetterSpacing", Some(9.0), "9"),
        ];
        for (key, stored, shown) in cases {
            let row = schema
                .rows()
                .find(|row| row.key().as_str() == key)
                .cloned()
                .unwrap();
            let values = stored_as(key, stored);
            let snapshot = crate::values::Snapshot::new(values);
            let state = crate::State::new(schema.clone(), snapshot, None, chord::Primary::Control);
            assert_eq!(state.field_text(&row), shown, "{key} {stored:?}");
        }
    }

    #[test]
    fn a_retired_default_reads_as_the_default_and_offers_no_reset() {
        let schema = crate::default_schema().unwrap();
        let row = schema
            .rows()
            .find(|row| row.key().as_str() == "terminalLineHeight")
            .cloned()
            .unwrap();
        let values = Values::default().with("terminalLineHeight", 1.2);
        let snapshot = crate::values::Snapshot::new(values);
        let state = crate::State::new(schema, snapshot, None, chord::Primary::Control);
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
