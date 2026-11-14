//! The text inputs of the rows on screen, by setting key.

use std::collections::BTreeMap;

use gpui::{App, Entity};
use ui::TextInput;

use crate::key::Key;
use crate::settings_state::SettingsState;

#[derive(Default)]
pub(crate) struct Fields {
    inputs: BTreeMap<Key, Entity<TextInput>>,
    subscriptions: Vec<gpui::Subscription>,
}

impl Fields {
    pub(crate) fn get(&self, key: &Key) -> Option<&Entity<TextInput>> {
        self.inputs.get(key)
    }

    pub(crate) fn add(
        &mut self,
        key: Key,
        input: Entity<TextInput>,
        subscriptions: Vec<gpui::Subscription>,
    ) {
        self.inputs.insert(key, input);
        self.subscriptions.extend(subscriptions);
    }

    pub(crate) fn sync(&self, state: &SettingsState, cx: &mut App) {
        state
            .schema()
            .rows()
            .filter_map(|row| Some((row, self.inputs.get(&row.key)?)))
            .for_each(|(row, input)| show(input, &state.field_text(row), cx));
    }
}

fn show(input: &Entity<TextInput>, text: &str, cx: &mut App) {
    input.update(cx, |input, cx| {
        if input.text() != text {
            input.set_text(text, cx);
        }
    });
}

#[cfg(test)]
#[path = "fields_tests.rs"]
mod tests;
