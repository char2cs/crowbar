use crate::schema::Control;
use crate::schema::Nudge;
use crate::schema::Row;
use crate::state::State;
use crate::state::effect::Effect;
use crate::values::Key;
use crate::values::Value;

impl State {
    pub fn toggle(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
        if !self.row_enabled(row) {
            return None;
        }
        let stored = self.document.value(&row.key).and_then(Value::as_bool)?;
        Some(self.write(&row.key, Value::Bool(!stored)))
    }

    pub fn choose(
        &mut self,
        row: &Row,
        index: usize,
    ) -> Option<Effect> {
        let choice = self.choices(row).into_iter().nth(index)?;
        Some(self.write(&row.key, Value::Text(choice.value)))
    }

    pub fn edit(
        &mut self,
        row: &Row,
        text: &str,
    ) {
        if self.row_enabled(row) {
            self.document.edit(&row.key, text);
        }
    }

    pub fn commit(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
        let text = self.document.draft(&row.key)?.to_owned();
        let parsed = row.control.parse(&text);
        let Ok(value) = parsed else {
            self.document.revert(&row.key);
            return None;
        };
        if self.document.value(&row.key) == Some(&value) {
            self.document.revert(&row.key);
            return None;
        }
        Some(self.write(&row.key, value))
    }

    pub fn revert(
        &mut self,
        row: &Row,
    ) {
        self.document.revert(&row.key);
    }

    pub fn nudge(
        &mut self,
        row: &Row,
        nudge: Nudge,
    ) -> Option<Effect> {
        let Control::Number { range, .. } = &row.control else {
            return None;
        };
        let typed = range.parse(&self.document.text(row));
        let current = typed.or_else(|| self.document.value(&row.key).and_then(Value::as_number))?;
        let next = range.stepped(current, nudge);
        Some(self.write(&row.key, Value::Number(next)))
    }

    pub fn reset(
        &mut self,
        row: &Row,
    ) -> Option<Effect> {
        let default = row.default.clone()?;
        if self.document.is_default(row) {
            self.document.revert(&row.key);
            return None;
        }
        Some(self.write(&row.key, default))
    }

    pub(crate) fn write(
        &mut self,
        key: &Key,
        value: Value,
    ) -> Effect {
        let effect = self.document.apply(key, value);
        self.refresh();
        effect
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::Issue;
    use crate::schema::Nudge;
    use crate::state::test_support::{gitignore_row, size_row, state, write};

    #[test]
    fn numbers_commit_once_clamped_and_never_while_typing() {
        let mut state = state();
        let row = size_row();
        state.edit(&row, "2");
        assert!(state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "2");
        assert_eq!(
            state.commit(&row),
            Some(write("fontSize", 10.0)),
            "clamped to the minimum on commit"
        );
        assert!(!state.is_dirty(&row));
        assert_eq!(state.field_text(&row), "10");
        state.edit(&row, "abc");
        assert_eq!(state.issue(&row), Some(Issue::NotANumber));
        assert_eq!(state.commit(&row), None);
        assert_eq!(
            state.field_text(&row),
            "10",
            "unparsable text is dropped, the stored value stays"
        );
        state.edit(&row, "10");
        assert_eq!(
            state.commit(&row),
            None,
            "typing the stored value back writes nothing"
        );
    }

    #[test]
    fn steppers_move_from_what_is_typed_and_stop_at_the_bounds() {
        let mut state = state();
        let row = size_row();
        assert_eq!(state.nudge(&row, Nudge::Up), Some(write("fontSize", 15.5)));
        state.edit(&row, "23.5");
        assert_eq!(
            state.nudge(&row, Nudge::Up),
            Some(write("fontSize", 24.0)),
            "starts from the typed value"
        );
        assert_eq!(
            state.nudge(&row, Nudge::Up),
            Some(write("fontSize", 24.0)),
            "the maximum holds"
        );
        assert_eq!(
            state.nudge(&gitignore_row(), Nudge::Up),
            None,
            "only numbers step"
        );
    }
}
