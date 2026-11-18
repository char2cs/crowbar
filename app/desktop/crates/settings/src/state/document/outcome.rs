//! What came back from the backend: acknowledgements, refusals and remote changes.

use crate::state::State;
use crate::values::Key;
use crate::values::Value;

impl State {
    /// The backend stored `key`.
    pub fn acknowledged(
        &mut self,
        key: &Key,
    ) {
        self.document.acknowledge(key);
    }

    /// The backend refused `key`: its value goes back.
    pub fn write_failed(
        &mut self,
        key: &Key,
    ) {
        self.document.fail(key);
        self.refresh();
    }

    /// The label of the row that stores `key`, or the key itself when no row does.
    pub fn label_of(
        &self,
        key: &Key,
    ) -> String {
        self.schema
            .rows()
            .find(|row| &row.key == key)
            .map_or_else(|| key.to_string(), |row| row.label.clone())
    }

    /// The backend reports a new value for `key`.
    pub fn remote_changed(
        &mut self,
        key: &Key,
        value: Value,
    ) {
        self.document.remote(key, value);
        self.refresh();
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::PageId;
    use crate::state::test_support::{custom_row, gitignore_row, state};
    use crate::values::Key;
    use crate::values::Value;

    #[test]
    fn a_failed_write_restores_the_value_and_names_the_row() {
        let mut state = state();
        let row = gitignore_row();
        assert!(state.toggle(&row).is_some());
        assert!(state.has_unsaved());
        state.write_failed(&Key::from("showIgnored"));
        assert_eq!(
            state.label_of(&Key::from("showIgnored")),
            "Respect .gitignore"
        );
        assert!(!state.switch_on(&row), "back to the default");
        assert!(!state.has_unsaved());
        assert!(state.toggle(&row).is_some());
        state.acknowledged(&Key::from("showIgnored"));
        assert!(!state.has_unsaved());
    }

    #[test]
    fn remote_changes_update_values_and_conditions() {
        let mut state = state();
        state.remote_changed(&Key::from("engine"), Value::from("custom"));
        state.select_page(&PageId::from("browser"));
        assert!(state.row_visible(&custom_row()));
    }
}
