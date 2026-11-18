use std::sync::Arc;

use anyhow::Context as _;
use browser::{AgentAnswer, PageUrl};
use browseragent::{PeerSession, TabCommands, TabInfo};
use layout::Command;
use protocol::PaneId;
use serde_json::{Value, json};

use super::ShellState;
use crate::browser_tabs::{address_of, browser_tabs, chat_tab};
use crate::pane_opener::open_in_tab;
use crate::pane_spec::PaneSpec;

const TAB_CLOSED: &str = "the tab was closed before the page loaded";

impl ShellState {
    pub(crate) fn browser_tabs(&self) -> Vec<TabInfo> {
        browser_tabs(self.history.layout(), &self.pages)
    }

    pub(crate) fn page_address(
        &self,
        pane: &PaneId,
    ) -> String {
        address_of(self.history.layout(), &self.pages, pane)
    }

    pub(crate) fn page_shown(
        &self,
        pane: &PaneId,
    ) -> Value {
        let state = self.pages.get(pane);
        json!({
            "url": state.map(browser::PageState::url),
            "title": state.and_then(|state| state.title()).unwrap_or_default(),
            "loading": false,
        })
    }

    pub(crate) fn open_agent_browser(
        &mut self,
        chat: &str,
        url: Option<&PageUrl>,
    ) -> anyhow::Result<PaneId> {
        let id = self.serials.next_browser();
        let spec = PaneSpec::agent_browser(&id, chat, url.map(PageUrl::as_str))
            .with_context(|| format!("the browser pane {id} could not be described"))?;
        let tab = chat_tab(self.history.layout(), chat)
            .with_context(|| format!("chat {chat} has no tab to hold the browser"))?;
        let opened = PaneId::new(&id).with_context(|| format!("{id} is not a pane id"))?;
        self.apply(vec![open_in_tab(tab, spec.instance(opened.clone()))]);
        self.history
            .layout()
            .pane(&opened)
            .map(|_| opened)
            .with_context(|| format!("the layout did not take pane {id}"))
    }

    pub(crate) fn close_pane(
        &mut self,
        pane: PaneId,
    ) {
        self.apply(vec![Command::ClosePane { pane }]);
    }

    fn fail_requests_of_closed_panes(&mut self) {
        let layout = self.history.layout();
        let is_open = |pane: &str| PaneId::new(pane).is_ok_and(|id| layout.pane(&id).is_some());
        self.replies.abandon_loads(is_open, TAB_CLOSED);
        let abandoned = self
            .pending_loads
            .take_closed(|pane| layout.pane(pane).is_some());
        for command in abandoned {
            let delivery = self
                .replies
                .complete(&command.id, AgentAnswer::Failed(TAB_CLOSED.into()));
            tracing::debug!(?delivery, id = %command.id, "a request waited for a pane that closed");
        }
    }

    pub(crate) fn peer_session(
        &mut self,
        commands: Arc<dyn TabCommands>,
    ) -> PeerSession {
        self.publish_tabs();
        PeerSession {
            tabs: self.tabs_out.subscribe(),
            commands,
            replies: Arc::clone(&self.replies),
        }
    }

    pub(crate) fn publish_tabs(&mut self) {
        self.pages.retain_open(self.history.layout());
        self.fail_requests_of_closed_panes();
        let tabs = self.browser_tabs();
        self.tabs_out
            .send_if_modified(|current| replace_if_changed(current, tabs));
    }
}

fn replace_if_changed(
    current: &mut Vec<TabInfo>,
    tabs: Vec<TabInfo>,
) -> bool {
    if *current == tabs {
        return false;
    }
    *current = tabs;
    true
}

#[cfg(test)]
mod tests {
    use browseragent::{AgentCommand, TabCommands};

    use super::*;
    use crate::test_support::{open_chat, shell_rig};

    #[derive(Debug)]
    struct Nobody;

    impl TabCommands for Nobody {
        fn run(
            &self,
            _: AgentCommand,
        ) {
        }
    }

    #[test]
    fn a_browser_cannot_open_in_a_chat_that_has_no_tab_and_the_reason_is_kept() {
        let mut rig = shell_rig();
        let error = rig.shell.open_agent_browser("c9", None).unwrap_err();
        assert!(format!("{error:#}").contains("c9"), "{error:#}");
    }

    #[test]
    fn the_peer_sees_the_tabs_as_they_open_change_and_close() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "c1");
        let session = rig.shell.peer_session(Arc::new(Nobody));
        assert!(session.tabs.borrow().is_empty());
        let url = PageUrl::parse("http://a.test/").unwrap();
        let pane = rig.shell.open_agent_browser("c1", Some(&url)).unwrap();
        assert!(pane.as_str().starts_with('b'), "a browser id");
        rig.shell.publish_tabs();
        let seen = session.tabs.borrow().clone();
        assert_eq!(
            seen.iter()
                .map(|tab| (tab.id.as_str(), tab.url.as_str()))
                .collect::<Vec<_>>(),
            [(pane.as_str(), "http://a.test/")]
        );
        rig.shell.close_pane(pane);
        rig.shell.publish_tabs();
        assert!(session.tabs.borrow().is_empty());
    }
}
