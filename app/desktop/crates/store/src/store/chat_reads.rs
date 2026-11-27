use crate::Store;

impl Store {
    pub fn chat_glyph(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::reads::chat_glyph::of(state, chat))
    }

    pub fn chat_reviews_branch(
        &self,
        chat: &str,
    ) -> bool {
        self.hub
            .read(|state| crate::reads::branch_review::of(state, chat))
    }

    pub fn chat_read_only(
        &self,
        chat: &str,
    ) -> bool {
        self.hub
            .read(|state| crate::reads::read_only::of(state, chat))
    }

    pub fn chat_project(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::chat_project::of(state, chat))
    }

    pub fn chat_workspace(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub.read(|state| {
            state
                .hosts
                .iter()
                .find_map(|slot| slot.store.chats.get(chat))
                .map(|record| record.workspace_id.clone())
                .filter(|workspace| !workspace.is_empty())
        })
    }

    pub fn chat_root(
        &self,
        chat: &str,
    ) -> Option<std::path::PathBuf> {
        self.hub
            .read(|state| crate::live::workspace_root::of_chat(state, chat))
    }

    pub fn chat_remote_host(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::workspace_root::remote_host_of(state, chat))
    }

    pub fn take_departed_chats(&self) -> Vec<String> {
        self.hub.quietly(|state| {
            let mut departed = std::mem::take(&mut state.left.chats);
            departed.extend(
                state
                    .hosts
                    .iter_mut()
                    .flat_map(|slot| slot.store.take_departed()),
            );
            departed
        })
    }

    pub fn take_departed_projects(&self) -> Vec<String> {
        self.hub.quietly(|state| {
            let mut departed = std::mem::take(&mut state.left.projects);
            departed.extend(
                state
                    .hosts
                    .iter_mut()
                    .flat_map(|slot| slot.store.take_departed_projects()),
            );
            departed
        })
    }

    pub fn chat_host(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::live::workspace_root::host_id_of(state, chat))
    }

    pub fn chat_row(
        &self,
        chat: &str,
    ) -> Option<sidebar::Row> {
        self.hub.read(|state| {
            state
                .hosts
                .iter()
                .find_map(|slot| crate::rows::chat_member::of(&slot.store, chat))
        })
    }
}

#[cfg(test)]
mod tests {
    use daemonapi::Frame;

    use crate::test_support::local_store;

    #[tokio::test]
    async fn a_loaded_chat_reads_its_project_and_its_tab_row() {
        let data = local_store(1);
        let frame = Frame::decode(
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c","projectId":"p","workspaceId":"w","working":true}}"#,
        )
        .unwrap();
        data.hub.edit(|state| state.apply("local", frame));
        assert_eq!(data.chat_project("c").as_deref(), Some("p"));
        assert_eq!(data.chat_workspace("c").as_deref(), Some("w"));
        assert_eq!(data.chat_workspace("gone"), None);
        let row = data.chat_row("c").unwrap();
        assert_eq!(row.activity, sidebar::Activity::Working, "its turn runs");
        assert!(
            data.chat_row("gone").is_none(),
            "an unknown chat has no row"
        );
    }

    const PROJECT: &str = r#"{"t":"entity","scope":"projects","kind":"project","id":"p","v":1,"data":{"id":"p","name":"Remote"}}"#;
    const CHAT: &str = r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c","projectId":"p"}}"#;

    #[tokio::test]
    async fn a_removed_host_takes_its_chats_and_projects_with_it() {
        let data = crate::test_support::paired_store("h1");
        let heard = [PROJECT, CHAT].map(|text| Frame::decode(text).unwrap());
        data.hub
            .edit(|state| heard.into_iter().for_each(|frame| state.apply("h1", frame)));
        data.fleet.forget("h1");
        assert_eq!(data.take_departed_chats(), ["c"], "its chats' views close");
        assert_eq!(
            data.take_departed_projects(),
            ["p"],
            "its spaces' views close"
        );
        assert!(data.take_departed_chats().is_empty(), "named once");
    }
}
