use std::ops::ControlFlow;

use crate::ending::Ending;
use crate::line::Line;
use crate::peer_session::PeerSession;
use crate::peer_stream::PeerStream;

pub(crate) async fn serve<S: PeerStream>(
    stream: &mut S,
    session: &PeerSession,
) -> Ending {
    let mut line = Line::new(session);
    let mut flow = ControlFlow::Continue(());
    while flow.is_continue() {
        flow = line.step(stream).await;
    }
    flow.break_value().unwrap_or(Ending::Lost)
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use browser_nav::AgentAnswer;
    use serde_json::{Value, json};
    use tokio::sync::watch;

    use super::*;
    use crate::agent_op::AgentOp;
    use crate::replies::Replies;
    use crate::tab_commands::TabCommands;
    use crate::tab_info::TabInfo;
    use crate::test_support::{Daemon, RecordingShell, line, tab};

    struct Rig {
        daemon: Daemon,
        tabs: watch::Sender<Vec<TabInfo>>,
        shell: Arc<RecordingShell>,
        replies: Arc<Replies>,
    }

    fn rig() -> Rig {
        let (mut stream, daemon) = line();
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let shell = Arc::new(RecordingShell::default());
        let replies = Arc::new(Replies::default());
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::clone(&shell) as Arc<dyn TabCommands>,
            replies: Arc::clone(&replies),
        };
        tokio::spawn(async move {
            serve(&mut stream, &session).await;
        });
        Rig {
            daemon,
            tabs,
            shell,
            replies,
        }
    }

    fn request(
        id: &str,
        op: &str,
        args: &Value,
    ) -> Value {
        json!({"type":"request","id":id,"op":op,"chatId":"c1","tab":"b1","args":args})
    }

    async fn settle() {
        tokio::task::yield_now().await;
        tokio::task::yield_now().await;
    }

    #[tokio::test(start_paused = true)]
    async fn a_hello_is_answered_with_the_tabs_and_a_change_is_reported_once() {
        let mut rig = rig();
        rig.daemon.say(&json!({"type":"hello"}));
        let first = rig.daemon.next().await;
        assert_eq!(first["tabs"][0]["url"], "http://a.test/");
        rig.tabs.send_replace(vec![tab("http://a.test/")]);
        rig.tabs.send_replace(vec![tab("http://b.test/")]);
        let second = rig.daemon.next().await;
        assert_eq!(
            second["tabs"][0]["url"], "http://b.test/",
            "an equal list is not repeated"
        );
        settle().await;
        assert!(rig.daemon.nothing_waits());
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_reaches_the_shell_and_the_shells_answer_goes_back_under_its_id() {
        let mut rig = rig();
        rig.daemon.say(&request("7", "click", &json!({"ref":"e1"})));
        let _tabs = rig.daemon.next().await;
        settle().await;
        let received = rig.shell.received();
        let named = received
            .first()
            .map(|c| (c.id.as_str(), c.chat_id.as_str(), c.tab.as_str()));
        assert_eq!(named, Some(("7", "c1", "b1")));
        assert!(matches!(received[0].op, AgentOp::Page { .. }));
        assert!(
            rig.replies
                .from_page("b1", "7", AgentAnswer::Done(json!("clicked")))
        );
        let answer = rig.daemon.next().await;
        assert_eq!(
            answer,
            json!({"type":"response","id":"7","ok":true,"result":"clicked"})
        );
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_the_shell_could_not_carry_out_is_answered_without_asking_it() {
        let mut rig = rig();
        rig.daemon.say(&request(
            "8",
            "navigate",
            &json!({"url":"file:///etc/passwd"}),
        ));
        let _tabs = rig.daemon.next().await;
        let answer = rig.daemon.next().await;
        assert_eq!(answer["ok"], false);
        assert!(
            answer["error"].as_str().unwrap().contains("file"),
            "{answer}"
        );
        assert_eq!(rig.shell.received().len(), 0);
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_nobody_answers_fails_at_its_limit() {
        let mut rig = rig();
        rig.daemon.say(&request("9", "snapshot", &json!({})));
        let _tabs = rig.daemon.next().await;
        let answer = rig.daemon.next().await;
        assert_eq!(
            (answer["ok"].clone(), answer["id"].clone()),
            (json!(false), json!("9"))
        );
        assert!(
            answer["error"].as_str().unwrap().contains("no answer"),
            "{answer}"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn a_connection_ends_as_lost_when_the_daemon_hangs_up_and_as_gone_when_the_window_does() {
        let (tabs, tabs_rx) = watch::channel(vec![tab("http://a.test/")]);
        let session = PeerSession {
            tabs: tabs_rx,
            commands: Arc::new(RecordingShell::default()),
            replies: Arc::new(Replies::default()),
        };
        let (mut stream, daemon) = line();
        drop(daemon);
        assert_eq!(serve(&mut stream, &session).await, Ending::Lost);
        let (mut stream, _daemon) = line();
        drop(tabs);
        assert_eq!(serve(&mut stream, &session).await, Ending::WindowGone);
    }
}
