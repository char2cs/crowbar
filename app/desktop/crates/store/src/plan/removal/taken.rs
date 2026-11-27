use std::collections::{BTreeMap, BTreeSet};

use serde_json::Value;
use sidebar::ProjectId;

use crate::live::Entities;
use crate::live::Outcome;
use crate::live::State;
use crate::reads;
use crate::rows::look::title_of;

const TAKEN_FIELD: &str = "takenChatIds";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Taken {
    chosen: BTreeSet<String>,
    titles: BTreeMap<String, String>,
}

impl Taken {
    pub(crate) fn before(
        state: &State,
        (project, row): (&str, &str),
    ) -> Self {
        let tree = reads::tree(state, &ProjectId::from(project));
        let chosen = tree
            .subtree(&row.into())
            .into_iter()
            .map(|id| id.as_str().to_owned())
            .chain(std::iter::once(row.to_owned()))
            .collect();
        let titles = reads::host_of(state, project)
            .map_or_else(BTreeMap::new, |slot| titles_of(&slot.store));
        Self { chosen, titles }
    }

    pub(crate) fn announce(
        &self,
        answer: &Value,
    ) -> Option<Outcome> {
        let listed = answer.get(TAKEN_FIELD).and_then(Value::as_array)?;
        let titles: Vec<String> = listed
            .iter()
            .filter_map(Value::as_str)
            .filter(|id| !self.chosen.contains(*id))
            .map(|id| self.title(id))
            .collect();
        (!titles.is_empty()).then_some(Outcome::AlsoRemoved { titles })
    }

    fn title(
        &self,
        id: &str,
    ) -> String {
        let held = self.titles.get(id).map_or("", String::as_str);
        title_of(held).to_owned()
    }
}

fn titles_of(store: &Entities) -> BTreeMap<String, String> {
    store
        .chats
        .values()
        .map(|chat| (chat.id.clone(), chat.title.clone()))
        .collect()
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::state;

    fn taken_by_deleting(
        row: &str,
        answer: &Value,
    ) -> Option<Outcome> {
        let (state, _) = state();
        Taken::before(&state, ("p", row)).announce(answer)
    }

    fn titled(titles: &[&str]) -> Outcome {
        let titles = titles.iter().map(|title| (*title).to_owned()).collect();
        Outcome::AlsoRemoved { titles }
    }

    #[test]
    fn only_chats_outside_the_selected_rows_are_named_by_their_title_before_the_delete() {
        let beyond = json!({"takenChatIds": ["bo", "t", "hc", "rdo", "gone"]});
        let named = taken_by_deleting("bo", &beyond);
        assert_eq!(
            named,
            Some(titled(&["Hello", "Untitled chat", "Untitled chat"])),
            "the branch and its thread were selected"
        );
        let inside = json!({"takenChatIds": ["t"]});
        assert_eq!(taken_by_deleting("bo", &inside), None, "all selected");
        let none = json!({"takenChatIds": []});
        assert_eq!(taken_by_deleting("bo", &none), None, "nothing taken");
        for odd in [json!(null), json!({}), json!({"takenChatIds": "hc"})] {
            assert_eq!(taken_by_deleting("bo", &odd), None, "{odd}");
        }
        let mixed = json!({"takenChatIds": [1, "hc"]});
        assert_eq!(taken_by_deleting("bo", &mixed), Some(titled(&["Hello"])));
    }
}
