//! The text fields of the rows on screen, by setting key.

use std::collections::BTreeMap;

use gpui::App;

use settings::Key;

use crate::frame::field::Field;

#[derive(Default)]
pub(crate) struct Fields {
    inputs: BTreeMap<Key, Field>,
    subscriptions: Vec<gpui::Subscription>,
}

impl Fields {
    pub(crate) fn get(
        &self,
        key: &Key,
    ) -> Option<&Field> {
        self.inputs.get(key)
    }

    pub(crate) fn add(
        &mut self,
        key: Key,
        input: Field,
        subscriptions: Vec<gpui::Subscription>,
    ) {
        self.inputs.insert(key, input);
        self.subscriptions.extend(subscriptions);
    }

    pub(crate) fn sync(
        &self,
        state: &settings::State,
        cx: &mut App,
    ) {
        state
            .schema()
            .rows()
            .filter_map(|row| Some((row, self.inputs.get(row.key())?)))
            .for_each(|(row, input)| show(input, &state.field_text(row), cx));
    }
}

fn show(
    input: &Field,
    text: &str,
    cx: &mut App,
) {
    if input.text(cx) == text {
        return;
    }
    input.set_text(text, cx);
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Value};

    use crate::test_support::{inspect, open};

    #[gpui::test]
    fn inputs_follow_the_state_when_another_window_changes_a_value(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        let key = Key::new("browserHomepage");
        let input = inspect(&harness, cx, |modal| modal.fields.get(&key).cloned()).unwrap();
        harness
            .backend
            .publish(&key, Value::Text("https://a.test".to_owned()));
        cx.run_until_parked();
        assert_eq!(cx.update(|_, cx| input.text(cx)), "https://a.test");
    }

    #[gpui::test]
    fn only_rows_on_a_visited_page_get_an_input(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        assert!(inspect(&harness, cx, |modal| modal
            .fields
            .get(&Key::new("browserHomepage"))
            .is_none()));
        assert!(inspect(&harness, cx, |modal| modal
            .fields
            .get(&Key::new("uiFontSize"))
            .is_some()));
    }
}
