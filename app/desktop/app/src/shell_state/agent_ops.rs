use browser_agent::{AgentCommand, AgentOp};
use browser_nav::{AgentAnswer, BrowserUrl};
use pane_host::BrowseAction;
use protocol::PaneId;
use serde_json::{Value, json};
use webview::ParentWindow;

use super::ShellState;
use super::agent_call::AgentCall;
use crate::browser_tabs::{chat_browser, chat_tab, owned_browser};

const CHAT_NOT_OPEN: &str =
    "this chat is not open in the Crowbar window, so it has no pane to hold a browser";
const NOT_OPENED: &str = "the browser could not be opened in this chat's pane";

pub(super) fn failed(text: impl Into<String>) -> AgentAnswer {
    AgentAnswer::Failed(text.into())
}

impl ShellState {
    pub(crate) fn run_agent(
        &mut self,
        command: &AgentCommand,
        parent: Option<ParentWindow>,
    ) {
        let call = AgentCall { command, parent };
        if let Some(answer) = self.agent_answer(&call) {
            self.replies.complete(&command.id, answer);
        }
        self.publish_tabs();
    }

    pub(crate) fn run_deferred(
        &mut self,
        pane: &PaneId,
        parent: Option<ParentWindow>,
    ) {
        let ready: Vec<AgentCommand> = self
            .pending_loads
            .take(pane)
            .into_iter()
            .filter(|command| self.replies.is_waiting(&command.id))
            .collect();
        for command in &ready {
            self.run_agent(command, parent);
        }
    }

    fn agent_answer(
        &mut self,
        call: &AgentCall<'_>,
    ) -> Option<AgentAnswer> {
        let command = call.command;
        if matches!(command.op, AgentOp::Open { .. }) {
            return self.agent_open(call);
        }
        let owned = owned_browser(self.history.layout(), &command.chat_id, &command.tab);
        let Some(pane) = owned else {
            return Some(failed(format!(
                "no such tab {:?} in this chat",
                command.tab
            )));
        };
        self.agent_tab(call, &pane)
    }

    fn agent_open(
        &mut self,
        call: &AgentCall<'_>,
    ) -> Option<AgentAnswer> {
        let AgentOp::Open { url, new_tab } = &call.command.op else {
            return None;
        };
        let chat = call.command.chat_id.as_str();
        if chat_tab(self.history.layout(), chat).is_none() {
            return Some(failed(CHAT_NOT_OPEN));
        }
        let existing = chat_browser(self.history.layout(), chat).filter(|_| !*new_tab);
        if let Some(pane) = existing {
            let base = json!({ "tab": pane.as_str(), "reused": true });
            return match url {
                Some(url) => self.agent_navigate(call, &pane, (url, base)),
                None => Some(AgentAnswer::Done(base)),
            };
        }
        let Some(pane) = self.open_agent_browser(chat, url.as_ref()) else {
            return Some(failed(NOT_OPENED));
        };
        let base = json!({ "tab": pane.as_str(), "reused": false });
        self.agent_first_load(call, &pane, base)
    }

    fn agent_first_load(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
        base: Value,
    ) -> Option<AgentAnswer> {
        if self.page_address(pane).is_empty() {
            return Some(AgentAnswer::Done(base));
        }
        if let Err(error) = self.bring_up(call.parent, pane) {
            return Some(failed(error.to_string()));
        }
        self.replies
            .after_load(&call.command.id, pane.as_str(), base);
        None
    }

    fn agent_tab(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> Option<AgentAnswer> {
        match &call.command.op {
            AgentOp::Close => {
                self.close_pane(pane.clone());
                Some(AgentAnswer::Done(Value::from("closed")))
            },
            AgentOp::Navigate { url } => self.agent_navigate(call, pane, (url, json!({}))),
            _ => self.agent_in_page(call, pane),
        }
    }

    fn agent_navigate(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
        (url, base): (&BrowserUrl, Value),
    ) -> Option<AgentAnswer> {
        if let Err(error) = self.bring_up(call.parent, pane) {
            return Some(failed(error.to_string()));
        }
        self.replies
            .after_load(&call.command.id, pane.as_str(), base);
        let went = self.host.browse(pane, &BrowseAction::Go(url.clone()));
        went.err().map(|error| failed(error.to_string()))
    }
}

#[cfg(test)]
mod tests {
    use browser_agent::AgentOp;
    use browser_nav::{AgentAnswer, BrowserUrl};
    use serde_json::json;
    use webview::{Call, ParentWindow};

    use super::*;
    use crate::test_support::{ShellRig, open_chat, settled, shell_rig};

    const WINDOW: Option<ParentWindow> = Some(ParentWindow::X11(9));

    fn command(
        id: &str,
        (chat, tab): (&str, &str),
        op: AgentOp,
    ) -> AgentCommand {
        AgentCommand {
            id: id.into(),
            chat_id: chat.into(),
            tab: tab.into(),
            op,
        }
    }

    fn open_op(url: Option<&str>) -> AgentOp {
        AgentOp::Open {
            url: url.map(|url| BrowserUrl::parse(url).unwrap()),
            new_tab: false,
        }
    }

    fn chat_rig() -> ShellRig {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "c1");
        rig
    }

    fn run(
        rig: &mut ShellRig,
        command: &AgentCommand,
    ) -> AgentAnswer {
        let wait = rig.shell.replies.register(&command.id, None);
        rig.shell.run_agent(command, WINDOW);
        settled(wait)
    }

    #[test]
    fn a_chat_that_is_not_open_has_no_pane_to_hold_a_browser() {
        let mut rig = chat_rig();
        let answer = run(&mut rig, &command("1", ("c9", ""), open_op(None)));
        assert_eq!(answer, AgentAnswer::Failed(CHAT_NOT_OPEN.into()));
    }

    #[test]
    fn a_blank_browser_opens_beside_its_chat_and_answers_at_once() {
        let mut rig = chat_rig();
        let answer = run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tabs = rig.shell.browser_tabs();
        let tab = tabs.first().expect("the browser is open");
        assert_eq!(
            answer,
            AgentAnswer::Done(json!({"tab": tab.id, "reused": false}))
        );
        assert_eq!(tab.chat_id, "c1");
        let again = run(&mut rig, &command("2", ("c1", ""), open_op(None)));
        assert_eq!(
            again,
            AgentAnswer::Done(json!({"tab": tab.id, "reused": true}))
        );
        assert_eq!(
            rig.shell.browser_tabs().len(),
            1,
            "the chat's browser was reused"
        );
    }

    #[test]
    fn an_opened_browser_answers_when_its_first_page_has_loaded() {
        let mut rig = chat_rig();
        let open = command("1", ("c1", ""), open_op(Some("http://a.test/")));
        let wait = rig.shell.replies.register("1", None);
        rig.shell.run_agent(&open, WINDOW);
        let pane = PaneId::new(rig.shell.browser_tabs()[0].id.as_str()).unwrap();
        assert!(rig.shell.replies.is_waiting("1"), "the page has not loaded");
        let created = rig
            .mock
            .calls()
            .into_iter()
            .any(|call| matches!(call, Call::Navigate { url, .. } if url.contains("a.test")));
        assert!(created, "the view loads the address");
        rig.shell.replies.loading(pane.as_str());
        rig.shell
            .replies
            .loaded(pane.as_str(), &json!({"url": "http://a.test/"}));
        let answer = settled(wait);
        assert_eq!(
            answer,
            AgentAnswer::Done(
                json!({"tab": pane.as_str(), "reused": false, "url": "http://a.test/"})
            )
        );
    }

    #[test]
    fn a_chat_cannot_reach_a_tab_of_another_chat_or_one_that_is_not_there() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let cases = [("c2", tab.as_str()), ("c1", "nope")];
        for (chat, id) in cases {
            let answer = run(&mut rig, &command("2", (chat, id), AgentOp::Close));
            assert!(
                matches!(answer, AgentAnswer::Failed(why) if why.contains("no such tab")),
                "{chat} {id}"
            );
        }
        assert_eq!(rig.shell.browser_tabs().len(), 1, "nothing was closed");
    }

    #[test]
    fn closing_a_tab_removes_its_pane() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let answer = run(&mut rig, &command("2", ("c1", &tab), AgentOp::Close));
        assert_eq!(answer, AgentAnswer::Done(json!("closed")));
        assert_eq!(rig.shell.browser_tabs(), []);
    }

    #[test]
    fn navigating_loads_the_address_in_the_view_and_waits_for_the_page() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let go = AgentOp::Navigate {
            url: BrowserUrl::parse("http://b.test/").unwrap(),
        };
        let wait = rig.shell.replies.register("2", None);
        rig.shell.run_agent(&command("2", ("c1", &tab), go), WINDOW);
        assert!(rig.shell.replies.is_waiting("2"));
        let went = rig
            .mock
            .calls()
            .into_iter()
            .any(|call| matches!(call, Call::Navigate { url, .. } if url.contains("b.test")));
        assert!(went, "the view was sent there");
        drop(wait);
    }

    #[test]
    fn without_a_window_to_hold_it_a_browser_cannot_be_brought_up() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let wait = rig.shell.replies.register("2", None);
        let go = AgentOp::Navigate {
            url: BrowserUrl::parse("http://b.test/").unwrap(),
        };
        rig.shell.run_agent(&command("2", ("c1", &tab), go), None);
        let answer = settled(wait);
        assert!(matches!(answer, AgentAnswer::Failed(_)), "{answer:?}");
    }
}
