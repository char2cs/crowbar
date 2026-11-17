use std::sync::Arc;

use browser_agent::{PeerSession, TabCommands, TabInfo};
use browser_nav::BrowserUrl;
use model::Command;
use protocol::PaneId;
use serde_json::{Value, json};

use super::ShellState;
use crate::browser_tabs::{address_of, browser_tabs, chat_tab};
use crate::pane_opener::open_in_tab;
use crate::pane_spec::PaneSpec;

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
            "url": state.map(browser_nav::PageState::url),
            "title": state.and_then(|state| state.title()).unwrap_or_default(),
            "loading": false,
        })
    }

    pub(crate) fn open_agent_browser(
        &mut self,
        chat: &str,
        url: Option<&BrowserUrl>,
    ) -> Option<PaneId> {
        let id = format!("b{}", self.serials.next_pane());
        let spec = PaneSpec::agent_browser(&id, chat, url.map(BrowserUrl::as_str)).ok()?;
        let tab = chat_tab(self.history.layout(), chat)?;
        let instance = spec.instance(PaneId::new(&id).ok()?);
        self.apply(vec![open_in_tab(tab, instance)]);
        let opened = PaneId::new(&id).ok()?;
        self.history.layout().pane(&opened).map(|_| opened)
    }

    pub(crate) fn close_pane(
        &mut self,
        pane: PaneId,
    ) {
        self.apply(vec![Command::ClosePane { pane }]);
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
    use browser_agent::{AgentCommand, TabCommands};

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
    fn the_peer_sees_the_tabs_as_they_open_change_and_close() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "c1");
        let session = rig.shell.peer_session(Arc::new(Nobody));
        assert!(session.tabs.borrow().is_empty());
        let url = BrowserUrl::parse("http://a.test/").unwrap();
        let pane = rig.shell.open_agent_browser("c1", Some(&url)).unwrap();
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
