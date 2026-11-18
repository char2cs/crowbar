use browser::{AgentAnswer, BrowserUrl};
use browseragent::{AgentCommand, AgentOp, Arming, Delivery};
use panehost::BrowseAction;
use protocol::PaneId;
use serde_json::{Value, json};
use webview::ParentWindow;

use super::ShellState;
use super::agent_call::AgentCall;
use super::agent_progress::AgentProgress;
use super::navigation_target::NavigationTarget;
use crate::browser_tabs::{chat_browser, chat_tab, owned_browser};

const CHAT_NOT_OPEN: &str =
    "this chat is not open in the Crowbar window, so it has no pane to hold a browser";
const NOT_OPENED: &str = "the browser could not be opened in this chat's pane";
const NOT_WAITING: &str = "the request is no longer waiting for an answer";
const TAB: &str = "tab";
const REUSED: &str = "reused";
const FRAGMENT: char = '#';

pub(super) fn failed(text: impl Into<String>) -> AgentProgress {
    AgentProgress::Answered(AgentAnswer::Failed(text.into()))
}

pub(super) fn done(value: Value) -> AgentProgress {
    AgentProgress::Answered(AgentAnswer::Done(value))
}

impl ShellState {
    pub(crate) fn run_agent(
        &mut self,
        command: &AgentCommand,
        parent: Option<ParentWindow>,
    ) {
        let call = AgentCall { command, parent };
        if let AgentProgress::Answered(answer) = self.agent_answer(&call) {
            let delivery = self.replies.complete(&command.id, answer);
            tracing::debug!(?delivery, id = %command.id, "an agent request was answered");
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
    ) -> AgentProgress {
        let command = call.command;
        if matches!(command.op, AgentOp::Open { .. }) {
            return self.agent_open(call);
        }
        let owned = owned_browser(self.history.layout(), &command.chat_id, &command.tab);
        let Some(pane) = owned else {
            return failed(format!("no such tab {:?} in this chat", command.tab));
        };
        self.agent_tab(call, &pane)
    }

    fn agent_open(
        &mut self,
        call: &AgentCall<'_>,
    ) -> AgentProgress {
        let AgentOp::Open { url, new_tab } = &call.command.op else {
            return failed("not an open request");
        };
        let chat = call.command.chat_id.as_str();
        if chat_tab(self.history.layout(), chat).is_none() {
            return failed(CHAT_NOT_OPEN);
        }
        let existing = chat_browser(self.history.layout(), chat).filter(|_| !*new_tab);
        if let Some(pane) = existing {
            let answer = json!({ TAB: pane.as_str(), REUSED: true });
            return match url {
                Some(url) => self.agent_navigate(call, &pane, NavigationTarget { url, answer }),
                None => done(answer),
            };
        }
        match self.open_agent_browser(chat, url.as_ref()) {
            Ok(pane) => self.agent_first_load(call, &pane),
            Err(error) => failed(format!("{NOT_OPENED}: {error:#}")),
        }
    }

    fn agent_first_load(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> AgentProgress {
        let answer = json!({ TAB: pane.as_str(), REUSED: false });
        let address = self.page_address(pane);
        if address.is_empty() || BrowserUrl::is_blank(&address) {
            return done(answer);
        }
        if let Err(error) = self.bring_up(call.parent, pane) {
            return failed(error.to_string());
        }
        self.wait_for_load(call, pane, answer)
    }

    fn agent_tab(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> AgentProgress {
        match &call.command.op {
            AgentOp::Close => {
                self.close_pane(pane.clone());
                done(Value::from("closed"))
            },
            AgentOp::Navigate { url } => {
                let answer = json!({});
                self.agent_navigate(call, pane, NavigationTarget { url, answer })
            },
            _ => self.agent_in_page(call, pane),
        }
    }

    fn agent_navigate(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
        target: NavigationTarget<'_>,
    ) -> AgentProgress {
        if let Err(error) = self.bring_up(call.parent, pane) {
            return failed(error.to_string());
        }
        let same_document = only_the_fragment_differs(&self.page_address(pane), target.url);
        if let Err(error) = self
            .host
            .browse(pane, &BrowseAction::Go(target.url.clone()))
        {
            return failed(error.to_string());
        }
        if same_document {
            return done(target.answer);
        }
        self.wait_for_load(call, pane, target.answer)
    }

    fn wait_for_load(
        &self,
        call: &AgentCall<'_>,
        pane: &PaneId,
        answer: Value,
    ) -> AgentProgress {
        match self
            .replies
            .after_load(&call.command.id, pane.as_str(), answer)
        {
            Arming::Armed => AgentProgress::Waiting,
            Arming::NotWaiting => failed(NOT_WAITING),
        }
    }

    pub(super) fn answer_from_page(
        &self,
        pane: &PaneId,
        id: &str,
        answer: AgentAnswer,
    ) {
        match self.replies.from_page(pane.as_str(), id, answer) {
            Delivery::Delivered => {},
            Delivery::NotWaiting => {
                tracing::debug!(id, "a page answered a request nobody awaits");
            },
            Delivery::Refused => {
                tracing::warn!(id, pane = %pane, "a page answered for another tab");
            },
        }
    }
}

fn only_the_fragment_differs(
    current: &str,
    target: &BrowserUrl,
) -> bool {
    let wanted = target.as_str();
    let moves_within = wanted.contains(FRAGMENT) && !current.is_empty();
    let without = |url: &str| url.split(FRAGMENT).next().map(str::to_owned);
    moves_within && without(current) == without(wanted)
}

#[cfg(test)]
mod tests {
    use browser::{AgentAnswer, BrowserUrl, PageUpdate};
    use browseragent::AgentOp;
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
        let mut seen = crate::outcome::Outcome::default();
        let landed = [("about:blank", true), ("http://a.test/", false)];
        for (url, still_waiting) in landed {
            let (loading, loaded) = (
                PageUpdate::Loading { url: url.into() },
                PageUpdate::Loaded { url: url.into() },
            );
            rig.shell.note_page(pane.clone(), loading, &mut seen);
            rig.shell.note_page(pane.clone(), loaded, &mut seen);
            let waiting = rig.shell.replies.is_waiting("1");
            assert_eq!(waiting, still_waiting, "{url}");
        }
        let answer = settled(wait);
        assert_eq!(
            answer,
            AgentAnswer::Done(
                json!({"tab": pane.as_str(), "reused": false, "url": "http://a.test/", "title": "", "loading": false})
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

    fn go(url: &str) -> AgentOp {
        AgentOp::Navigate {
            url: BrowserUrl::parse(url).unwrap(),
        }
    }

    fn pane_of(rig: &ShellRig) -> PaneId {
        PaneId::new(rig.shell.browser_tabs()[0].id.as_str()).unwrap()
    }

    fn page_loads(
        rig: &mut ShellRig,
        pane: &PaneId,
        url: &str,
    ) {
        let mut seen = crate::outcome::Outcome::default();
        let loading = PageUpdate::Loading { url: url.into() };
        let loaded = PageUpdate::Loaded { url: url.into() };
        rig.shell.note_page(pane.clone(), loading, &mut seen);
        rig.shell.note_page(pane.clone(), loaded, &mut seen);
    }

    #[test]
    fn navigating_loads_the_address_in_the_view_and_answers_when_the_page_has_loaded() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let wait = rig.shell.replies.register("2", None);
        let go_b = command("2", ("c1", &tab), go("http://b.test/"));
        rig.shell.run_agent(&go_b, WINDOW);
        assert!(rig.shell.replies.is_waiting("2"));
        let went = rig
            .mock
            .calls()
            .into_iter()
            .any(|call| matches!(call, Call::Navigate { url, .. } if url.contains("b.test")));
        assert!(went, "the view was sent there");
        let pane = pane_of(&rig);
        page_loads(&mut rig, &pane, "http://b.test/");
        let answer = settled(wait);
        assert!(matches!(answer, AgentAnswer::Done(_)), "{answer:?}");
    }

    #[test]
    fn a_navigation_that_only_moves_the_fragment_answers_at_once() {
        let mut rig = chat_rig();
        let open = command("1", ("c1", ""), open_op(Some("http://a.test/page")));
        let first = rig.shell.replies.register("1", None);
        rig.shell.run_agent(&open, WINDOW);
        let pane = pane_of(&rig);
        page_loads(&mut rig, &pane, "http://a.test/page");
        drop(first);
        let answer = run(
            &mut rig,
            &command("2", ("c1", pane.as_str()), go("http://a.test/page#top")),
        );
        assert!(matches!(answer, AgentAnswer::Done(_)), "{answer:?}");
    }

    #[test]
    fn a_navigation_waiting_for_a_tab_that_is_closed_fails_without_waiting_for_the_limit() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let wait = rig.shell.replies.register("2", None);
        let go_b = command("2", ("c1", &tab), go("http://b.test/"));
        rig.shell.run_agent(&go_b, WINDOW);
        rig.shell.close_pane(PaneId::new(&tab).unwrap());
        rig.shell.publish_tabs();
        let answer = settled(wait);
        assert!(
            matches!(&answer, AgentAnswer::Failed(why) if why.contains("closed")),
            "{answer:?}"
        );
    }

    #[test]
    fn without_a_window_to_hold_it_a_browser_cannot_be_brought_up() {
        let mut rig = chat_rig();
        run(&mut rig, &command("1", ("c1", ""), open_op(None)));
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let wait = rig.shell.replies.register("2", None);
        let go_b = command("2", ("c1", &tab), go("http://b.test/"));
        rig.shell.run_agent(&go_b, None);
        let answer = settled(wait);
        assert!(matches!(answer, AgentAnswer::Failed(_)), "{answer:?}");
    }
}
