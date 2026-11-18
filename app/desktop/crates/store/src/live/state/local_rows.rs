//! The creates this shell started itself, shown until their chats arrive.

use crate::live::held::PendingRow;
use crate::live::state::State;

impl State {
    pub(crate) fn add_local(
        &mut self,
        row: PendingRow,
    ) {
        let arrived = |id: &str| {
            self.hosts
                .iter()
                .any(|slot| slot.store.chats.get(id).is_some())
        };
        let live: Vec<PendingRow> = self
            .local_rows
            .iter()
            .filter(|held| !arrived(&held.id) && held.id != row.id)
            .cloned()
            .collect();
        self.local_rows = live;
        self.local_rows.push(row);
    }

    pub(crate) fn fail_local(
        &mut self,
        id: &str,
        reason: &str,
    ) {
        for row in self.local_rows.iter_mut().filter(|row| row.id == id) {
            row.failure = Some(reason.to_owned());
        }
    }

    pub(crate) fn drop_local(
        &mut self,
        id: &str,
    ) -> bool {
        let before = self.local_rows.len();
        self.local_rows.retain(|row| row.id != id);
        self.local_rows.len() != before
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    fn row(id: &str) -> PendingRow {
        PendingRow {
            id: id.into(),
            label: "New".into(),
            scope: String::new(),
            parent: String::new(),
            failure: None,
            operation: None,
        }
    }

    #[test]
    fn local_creates_are_added_failed_dropped_and_forgotten_once_their_chat_arrives() {
        let (mut state, _) = state();
        state.add_local(row("hc"));
        state.add_local(row("x"));
        assert_eq!(
            state.local_rows.len(),
            1,
            "the create whose chat arrived is forgotten"
        );
        state.fail_local("x", "no agent");
        assert_eq!(state.local_rows[0].failure.as_deref(), Some("no agent"));
        assert!(state.drop_local("x"));
        assert!(!state.drop_local("x"));
    }
}
