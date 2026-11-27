use browser::{PageState, PageUrl};
use panehost::BrowseAction;
use protocol::PaneId;
use webview::ParentWindow;

use super::agent_call::AgentCall;
use super::agent_ops::{done, failed};
use super::agent_progress::AgentProgress;
use super::page_step::PageStep;
use crate::panes::browser::page_shot::png_of;
use crate::shell::shell_state::ShellState;
use crate::shell::shell_state::agent::view_life::ViewLife;

const NO_PAGE: &str = "this tab has no page yet; use browser_navigate first";
const NOT_READABLE: &str = "the page could not be read back as a picture";

impl ShellState {
    pub(crate) fn agent_in_page(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> AgentProgress {
        let address = self.page_address(pane);
        if address.is_empty() || PageUrl::is_blank(&address) {
            return failed(NO_PAGE);
        }
        match self.bring_up(call.parent, pane) {
            Err(error) => return failed(error.to_string()),
            Ok(ViewLife::Fresh) => return self.defer_until_loaded(call, pane),
            Ok(ViewLife::Live) => {},
        }
        if self.pages.get(pane).is_some_and(PageState::is_loading) {
            return self.defer_until_loaded(call, pane);
        }
        self.agent_page_step(call, pane)
    }

    fn defer_until_loaded(
        &mut self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> AgentProgress {
        self.pending_loads.defer(pane.clone(), call.command.clone());
        AgentProgress::Waiting
    }

    fn agent_page_step(
        &self,
        call: &AgentCall<'_>,
        pane: &PaneId,
    ) -> AgentProgress {
        let sent = match PageStep::of(call.command) {
            PageStep::Browse(action, said) => {
                return match self.host.browse(pane, &action) {
                    Ok(()) => done(said.into()),
                    Err(error) => failed(error.to_string()),
                };
            },
            PageStep::Shot => return self.agent_shot(pane),
            PageStep::Script(js) => self.host.browse(pane, &BrowseAction::Script(js)),
            PageStep::Refused(why) => return failed(why),
        };
        match sent {
            Ok(()) => AgentProgress::Waiting,
            Err(error) => failed(error.to_string()),
        }
    }

    fn agent_shot(
        &self,
        pane: &PaneId,
    ) -> AgentProgress {
        match self.host.capture_browser(pane) {
            Ok(Some(snapshot)) => match png_of(&snapshot) {
                Ok(shot) => done(shot),
                Err(error) => failed(format!("{NOT_READABLE}: {error:#}")),
            },
            Ok(None) => failed(NOT_READABLE),
            Err(error) => failed(error.to_string()),
        }
    }

    pub(crate) fn bring_up(
        &mut self,
        parent: Option<ParentWindow>,
        pane: &PaneId,
    ) -> panehost::Result<ViewLife> {
        if self.host.has_view(pane) {
            return Ok(ViewLife::Live);
        }
        let parent = parent.ok_or_else(|| panehost::Error::NotShown(pane.clone()))?;
        self.host
            .wake_browser(parent, pane, self.history.layout())?;
        Ok(ViewLife::Fresh)
    }
}

#[cfg(test)]
mod tests {
    use browser::{AgentAnswer, PageUpdate, PageUrl};
    use browseragent::{AgentCommand, AgentOp, Delivery, DomOp};
    use serde_json::json;
    use webview::{Call, ParentWindow};

    use super::*;
    use crate::test_support::{ShellRig, open_chat, settled, shell_rig};

    const WINDOW: Option<ParentWindow> = Some(ParentWindow::X11(9));

    fn command(
        id: &str,
        tab: &str,
        op: AgentOp,
    ) -> AgentCommand {
        AgentCommand {
            id: id.into(),
            chat_id: "c1".into(),
            tab: tab.into(),
            op,
        }
    }

    fn rig_with_a_page() -> (ShellRig, String) {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "c1");
        let url = PageUrl::parse("http://a.test/").unwrap();
        let open = AgentOp::Open {
            url: Some(url),
            new_tab: false,
        };
        rig.shell.run_agent(&command("1", "", open), WINDOW);
        let tab = rig.shell.browser_tabs()[0].id.clone();
        let pane = PaneId::new(&tab).unwrap();
        rig.shell.replies.loading(pane.as_str());
        rig.shell.replies.loaded(pane.as_str(), &json!({}));
        (rig, tab)
    }

    fn scripts(rig: &ShellRig) -> Vec<String> {
        rig.mock
            .calls()
            .into_iter()
            .filter_map(|call| match call {
                Call::Eval { script, .. } => Some(script),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_page_operation_runs_its_script_in_the_view_and_the_page_answers_it() {
        let (mut rig, tab) = rig_with_a_page();
        let click = AgentOp::Page {
            op: DomOp::Click,
            args: json!({"ref": "e3"}),
        };
        let wait = rig.shell.replies.register("2", Some(&tab));
        rig.shell.run_agent(&command("2", &tab, click), WINDOW);
        assert!(
            scripts(&rig)
                .iter()
                .any(|js| js.contains("e3") && js.contains("(\"2\",")),
            "{:?}",
            scripts(&rig)
        );
        let delivered = rig
            .shell
            .replies
            .from_page(&tab, "2", AgentAnswer::Done(json!("ok")));
        assert_eq!(delivered, Delivery::Delivered);
        assert_eq!(settled(wait), AgentAnswer::Done(json!("ok")));
    }

    #[test]
    fn a_page_operation_on_a_view_that_is_still_loading_waits_for_the_load() {
        let (mut rig, tab) = rig_with_a_page();
        let pane = PaneId::new(&tab).unwrap();
        let mut seen = crate::shell::shell_state::host::outcome::Outcome::default();
        let loading = PageUpdate::Loading {
            url: "http://a.test/next".into(),
        };
        rig.shell.note_page(pane.clone(), loading, &mut seen);
        let click = AgentOp::Page {
            op: DomOp::Click,
            args: json!({"ref": "e3"}),
        };
        let wait = rig.shell.replies.register("2", Some(&tab));
        rig.shell.run_agent(&command("2", &tab, click), WINDOW);
        let ran = |rig: &ShellRig| scripts(rig).iter().any(|js| js.contains("e3"));
        assert!(!ran(&rig), "the view is mid-navigation");
        let loaded = PageUpdate::Loaded {
            url: "http://a.test/next".into(),
        };
        rig.shell.note_page(pane.clone(), loaded, &mut seen);
        rig.shell.run_deferred(&pane, WINDOW);
        assert!(ran(&rig), "the load ended, so the operation ran");
        let said = AgentAnswer::Done(json!("ok"));
        rig.shell.replies.from_page(&tab, "2", said.clone());
        assert_eq!(settled(wait), said);
    }

    #[test]
    fn history_moves_answer_at_once_and_a_page_that_never_loaded_is_refused() {
        let (mut rig, tab) = rig_with_a_page();
        let cases = [
            (AgentOp::Back, "went back", "history.back()"),
            (AgentOp::Forward, "went forward", "history.forward()"),
            (AgentOp::Reload, "reloading", "location.reload()"),
        ];
        for (op, said, script) in cases {
            let wait = rig.shell.replies.register("2", None);
            rig.shell.run_agent(&command("2", &tab, op), WINDOW);
            assert_eq!(settled(wait), AgentAnswer::Done(json!(said)));
            assert!(scripts(&rig).iter().any(|js| js == script), "{script}");
        }

        let mut blank = shell_rig();
        open_chat(&mut blank.shell, "c1");
        let open = AgentOp::Open {
            url: None,
            new_tab: false,
        };
        blank.shell.run_agent(&command("1", "", open), WINDOW);
        let tab = blank.shell.browser_tabs()[0].id.clone();
        let wait = blank.shell.replies.register("2", None);
        blank
            .shell
            .run_agent(&command("2", &tab, AgentOp::Back), WINDOW);
        let answer = settled(wait);
        assert_eq!(answer, AgentAnswer::Failed(NO_PAGE.into()));
    }

    #[test]
    fn a_picture_the_view_cannot_give_is_a_failure_that_says_so() {
        let (mut rig, tab) = rig_with_a_page();
        let wait = rig.shell.replies.register("2", None);
        rig.shell
            .run_agent(&command("2", &tab, AgentOp::Screenshot), WINDOW);
        assert_eq!(settled(wait), AgentAnswer::Failed(NOT_READABLE.into()));
    }
}
