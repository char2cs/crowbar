//! What the current values show: rows, blocks, options and descriptions.

use crate::keys::Shortcut;
use crate::schema::ChoiceSource;
use crate::schema::Control;
use crate::schema::Row;
use crate::settings_state::SettingsState;
use crate::values::Choice;

impl SettingsState {
    /// Whether the search leaves `row` visible.
    pub fn row_visible(
        &self,
        row: &Row,
    ) -> bool {
        self.visibility.row(row)
    }

    /// Whether the search leaves `shortcut` visible.
    pub fn shortcut_visible(
        &self,
        shortcut: &Shortcut,
    ) -> bool {
        self.visibility.shortcut(shortcut)
    }

    /// Whether the search leaves the non-row block `index` of the selected page visible.
    pub fn block_visible(
        &self,
        index: usize,
    ) -> bool {
        self.visibility.block(self.navigation.active(), index)
    }

    /// The description under the label: the invalid-value text while the field is invalid.
    pub fn description<'a>(
        &self,
        row: &'a Row,
    ) -> Option<&'a str> {
        if self.issue(row).is_some() {
            return row
                .invalid_description
                .as_deref()
                .or(row.description.as_deref());
        }
        if let Control::Button(spec) = &row.control
            && !self.button_enabled(spec)
        {
            return spec.disabled_note.as_deref().or(row.description.as_deref());
        }
        row.description.as_deref()
    }

    /// The options of the select `row`.
    pub fn choices(
        &self,
        row: &Row,
    ) -> Vec<Choice> {
        let Control::Select { source, .. } = &row.control else {
            return Vec::new();
        };
        match source {
            ChoiceSource::Fixed(choices) => choices.clone(),
            ChoiceSource::Provided(name) => self.choices.get(name).cloned().unwrap_or_default(),
        }
    }

    /// The index of the option the select `row` currently stores.
    pub fn selected_index(
        &self,
        row: &Row,
    ) -> Option<usize> {
        let stored = self.document.value(&row.key)?.as_text()?;
        self.choices(row)
            .iter()
            .position(|choice| choice.value == stored)
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::PageId;
    use crate::settings_state::test_support::state_fixtures::{
        custom_row, engine_row, state, write,
    };

    #[test]
    fn selects_store_the_option_value_and_reveal_dependent_rows() {
        let mut state = state();
        let (engine, custom) = (engine_row(), custom_row());
        state.select_page(&PageId::from("browser"));
        assert_eq!(state.selected_index(&engine), Some(0));
        assert!(
            !state.row_visible(&custom),
            "the custom URL row waits for the custom engine"
        );
        assert_eq!(state.choose(&engine, 1), Some(write("engine", "custom")));
        assert_eq!(state.selected_index(&engine), Some(1));
        assert!(state.row_visible(&custom));
        assert_eq!(
            state.choose(&engine, 9),
            None,
            "an index past the options does nothing"
        );
    }
}
