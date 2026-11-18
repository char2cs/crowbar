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
    use std::time::Duration;

    use browser::AgentAnswer;
    use serde_json::{Value, json};
    use tokio::sync::watch;

    use super::*;
    use crate::agent_op::AgentOp;
    use crate::delivery::Delivery;
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
        let more = tokio::time::timeout(Duration::from_millis(1), rig.daemon.next()).await;
        assert!(more.is_err(), "nothing more was reported");
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_reaches_the_shell_and_the_shells_answer_goes_back_under_its_id() {
        let mut rig = rig();
        rig.daemon.say(&request("7", "click", &json!({"ref":"e1"})));
        let _tabs = rig.daemon.next().await;
        let received = rig.shell.first().await;
        let named = (
            received.id.as_str(),
            received.chat_id.as_str(),
            received.tab.as_str(),
        );
        assert_eq!(named, ("7", "c1", "b1"));
        assert!(matches!(received.op, AgentOp::Page { .. }));
        let delivered = rig
            .replies
            .from_page("b1", "7", AgentAnswer::Done(json!("clicked")));
        assert_eq!(delivered, Delivery::Delivered);
        let answer = rig.daemon.next().await;
        assert_eq!(
            answer,
            json!({"type":"response","id":"7","ok":true,"result":"clicked"})
        );
    }

    const CASES: &[(&str, &str, &str)] = &[
        ("navigate", r#"{"url":"file:///etc/passwd"}"#, "file"),
        ("snapshot", "{}", "no answer"),
    ];

    async fn fails_under_its_id(failure: &(&str, &str, &str)) {
        let (op, args, mentions) = *failure;
        let mut rig = rig();
        let args: Value = serde_json::from_str(args).unwrap();
        rig.daemon.say(&request("9", op, &args));
        let _tabs = rig.daemon.next().await;
        let answer = rig.daemon.next().await;
        assert_eq!((&answer["ok"], &answer["id"]), (&json!(false), &json!("9")));
        let error = answer["error"].as_str().unwrap();
        assert!(error.contains(mentions), "{op}: {answer}");
    }

    async fn every_failure() {
        for failure in CASES {
            fails_under_its_id(failure).await;
        }
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_that_cannot_be_carried_out_or_is_never_answered_fails_under_its_id() {
        every_failure().await;
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
