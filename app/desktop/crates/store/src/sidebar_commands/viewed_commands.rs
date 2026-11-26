//! The review's viewed marks: one owner, written from the Changes panel and from review panes.

use std::collections::BTreeMap;

use crate::live::Outcome;
use crate::reads::files::workspace_of_chat;
use crate::sidebar_commands::SidebarCommands;

/// A workspace's viewed marks: path to the signature the file had when it was marked.
pub type ViewedMarks = BTreeMap<String, String>;

impl SidebarCommands {
    /// Marks files of `chat`'s workspace viewed with their signatures, or clears the marks where
    /// a file has no signature, saves them and tells the shell. A chat the store does not know
    /// changes nothing.
    pub fn set_review_viewed(
        &self,
        chat: &str,
        changes: &[(String, Option<String>)],
    ) {
        let applied = self.hub.edit(|state| {
            let workspace = workspace_of_chat(state, chat)?;
            let viewed = state.viewed.apply(&workspace, changes);
            state.viewed.save(self.files.as_ref());
            Some(Outcome::ReviewViewed { workspace, viewed })
        });
        if let Some(outcome) = applied {
            (self.outcome)(outcome);
        }
    }

    /// The workspace `chat` works in and that workspace's viewed marks; `None` until the chat
    /// is known.
    pub fn review_viewed(
        &self,
        chat: &str,
    ) -> Option<(String, ViewedMarks)> {
        self.hub.read(|state| {
            let workspace = workspace_of_chat(state, chat)?;
            let viewed = state.viewed.of(&workspace);
            Some((workspace, viewed))
        })
    }

    pub(crate) fn set_followed_viewed(
        &self,
        changes: &[(String, Option<String>)],
    ) {
        let Some(chat) = self.hub.read(|state| state.panels.chat.clone()) else {
            return;
        };
        self.set_review_viewed(&chat, changes);
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;
    use std::sync::Arc;

    use sidebar::{Command, Sink};

    use super::*;
    use crate::test_support::{FakeFiles, followed_commands, recorder};

    #[tokio::test]
    async fn a_mark_from_either_surface_lands_in_the_chats_workspace_and_is_told() {
        let (outcome, heard) = recorder();
        let (commands, _) = followed_commands(outcome, &Arc::new(FakeFiles::default()));
        commands
            .dispatch(Command::SetFilesViewed {
                files: vec![("a.ts".into(), Some("M||1|0".into()))],
            })
            .unwrap();
        commands.set_review_viewed("hc", &[("b.ts".into(), Some("A||2|0".into()))]);
        commands.set_review_viewed("unknown", &[("c.ts".into(), Some("x".into()))]);
        commands.set_review_viewed("hc", &[("a.ts".into(), None)]);
        let last = heard.lock().unwrap().last().cloned();
        let expected = Outcome::ReviewViewed {
            workspace: "hw".into(),
            viewed: BTreeMap::from([("b.ts".into(), "A||2|0".into())]),
        };
        assert_eq!(last, Some(expected));
        assert_eq!(
            heard.lock().unwrap().len(),
            3,
            "the unknown chat changed nothing"
        );
        let read = commands.review_viewed("hc");
        let marks = BTreeMap::from([("b.ts".into(), "A||2|0".into())]);
        assert_eq!(read, Some(("hw".into(), marks)));
    }
}
