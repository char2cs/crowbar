use layout::{Command, SpaceId, Tab};

use super::ShellState;
use crate::panes::kinds::core_panes::THREAD_ID;

impl ShellState {
    pub(crate) fn adopt_unowned(&mut self) {
        let unowned = self.history.layout().tabs().iter();
        let unowned = unowned.filter(|tab| tab.space().is_empty());
        let adoptions: Vec<Command> = unowned.filter_map(|tab| self.adoption(tab)).collect();
        if adoptions.is_empty() {
            return;
        }
        self.run(adoptions);
    }

    pub(crate) fn closing_spaces(
        &self,
        spaces: &[String],
    ) -> Vec<Command> {
        let gone = |tab: &&Tab| spaces.iter().any(|space| space == tab.space());
        let tabs = self.history.layout().tabs().iter().filter(gone);
        tabs.map(|tab| Command::CloseTab { tab: tab.id() })
            .collect()
    }

    fn adoption(
        &self,
        tab: &Tab,
    ) -> Option<Command> {
        let chats: Vec<&str> = tab
            .pane_ids()
            .iter()
            .filter_map(|pane| tab.pane(pane)?.params.get(THREAD_ID))
            .collect();
        let known = chats.iter().find_map(|chat| self.space.of_chat(chat));
        let chatless = chats.is_empty().then(|| self.space.active());
        let space = known.or(chatless).filter(|space| !space.is_empty())?;
        Some(Command::AdoptSpace {
            tab: tab.id(),
            space: SpaceId::new(space),
        })
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;
    use std::rc::Rc;
    use std::sync::Arc;

    use serde_json::json;
    use webview::MockPlatform;

    use super::*;
    use crate::test_support::{FakeSpace, LAYOUT_PATH, MemoryFiles, mock_host};

    fn leaf(
        chat: Option<&str>,
        tabs: &[&str],
    ) -> serde_json::Value {
        json!({"node": "leaf", "chat": chat, "tabs": tabs, "active": tabs.first(), "pinned": 0,
            "editorOpen": chat.is_none(), "chatSelected": chat.is_some()})
    }

    fn stored_tab(
        (id, pane): (u64, &str),
        (type_id, params): (&str, serde_json::Value),
    ) -> serde_json::Value {
        let chat = (type_id == "core.chat").then_some(pane);
        let tabs: Vec<&str> = chat.map_or_else(|| vec![pane], |_| Vec::new());
        let instance = json!({"paneId": pane, "type": type_id, "params": params, "state": null});
        json!({"id": id, "title": "", "panes": {pane: instance}, "tree": leaf(chat, &tabs),
            "focused": pane})
    }

    fn v3() -> String {
        let tabs = [
            stored_tab((1, "a"), ("core.chat", json!({"threadId": "c2"}))),
            stored_tab((2, "b"), ("core.terminal", json!({"terminalId": "t1"}))),
            stored_tab((3, "c"), ("core.chat", json!({"threadId": "unknown"}))),
        ];
        json!({"version": 3, "active": 1, "tabs": tabs}).to_string()
    }

    fn spaces(shell: &ShellState) -> Vec<String> {
        shell
            .layout()
            .tabs()
            .iter()
            .map(|tab| tab.space().to_owned())
            .collect()
    }

    #[test]
    fn a_stored_tab_takes_its_chat_s_space_and_a_chatless_one_the_active_space() {
        let files = Arc::new(MemoryFiles::default());
        files.seed(Path::new(LAYOUT_PATH), &v3());
        let space = Rc::new(FakeSpace::default());
        space.select("p1");
        space.know(("c2", "p2"));
        let port = Rc::<FakeSpace>::clone(&space);
        let mock = Rc::new(MockPlatform::default());
        let mut shell = ShellState::new(mock_host(&mock), files.store(), port).unwrap();
        shell.adopt_unowned();
        assert_eq!(spaces(&shell), ["p2", "p1", ""], "the unknown chat waits");
        let shown = shell.layout().showing("p2").map(Tab::id);
        assert_eq!(shown, Some(layout::TabId(1)), "p2 shows the old active tab");
        space.know(("unknown", "p3"));
        shell.adopt_unowned();
        assert_eq!(spaces(&shell), ["p2", "p1", "p3"], "adopted once known");
    }

    #[test]
    fn a_deleted_space_closes_every_tab_of_its_own_and_no_other() {
        let mut rig = crate::test_support::shell_rig();
        rig.space.select("p1");
        crate::test_support::open_chat(&mut rig.shell, "t1");
        rig.space.select("p2");
        crate::test_support::open_chat(&mut rig.shell, "t2");
        let closing = rig.shell.closing_spaces(&["p2".to_owned()]);
        rig.shell.apply(closing);
        assert_eq!(spaces(&rig.shell), ["p1"], "p2's tab closed, p1's stays");
    }
}
