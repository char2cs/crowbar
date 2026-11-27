use crate::schema::ChoiceSource;
use crate::schema::Control;
use crate::schema::Row;
use crate::state::State;
use crate::values::Choice;

impl State {
    pub fn row_visible(
        &self,
        row: &Row,
    ) -> bool {
        self.visibility.row(row)
    }

    pub fn block_visible(
        &self,
        index: usize,
    ) -> bool {
        self.visibility.block(self.navigation.active(), index)
    }

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
        let worded = row
            .worded
            .as_ref()
            .filter(|(rule, _)| rule.holds(&self.scope()));
        worded.map_or(row.description.as_deref(), |(_, text)| Some(text.as_str()))
    }

    pub fn row_enabled(
        &self,
        row: &Row,
    ) -> bool {
        row.frozen
            .as_ref()
            .is_none_or(|rule| !rule.holds(&self.scope()))
    }

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
    use crate::schema::{Condition, PageId};
    use crate::state::test_support::{custom_row, engine_row, state, write};

    #[test]
    fn a_frozen_row_says_why_and_takes_no_typing_while_its_condition_holds() {
        let picked = || Condition::equals("engine", "duckduckgo");
        let frozen = custom_row()
            .describe_while((picked(), "Pick Custom to change it."))
            .frozen_while(picked());
        let mut state = state();
        assert!(!state.row_enabled(&frozen), "frozen");
        assert_eq!(
            state.description(&frozen),
            Some("Pick Custom to change it.")
        );
        state.edit(&frozen, "https://a.example/?q=%s");
        assert_eq!(state.commit(&frozen), None, "the typing was refused");
        state.choose(&engine_row(), 1);
        assert!(state.row_enabled(&frozen), "free again");
        assert_eq!(state.description(&frozen), Some("Search address"));
    }

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
