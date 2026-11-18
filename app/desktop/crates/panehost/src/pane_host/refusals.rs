use protocol::{
    DisplayText, ErrorCode, MethodName, PaneId, RequestId, RpcError, RpcOutcome, RpcResponse,
    ShellFrame,
};

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::slot::HostEvent;
use crate::slot::IpcNotice;
use crate::slot::MAX_MESSAGE_BYTES;

const MAX_CONSECUTIVE_VIOLATIONS: u32 = 8;
const MAX_REASON_CHARS: usize = 256;

fn reason(text: &str) -> Result<DisplayText> {
    Ok(DisplayText::new(
        text.chars().take(MAX_REASON_CHARS).collect::<String>(),
    )?)
}

impl PaneHost {
    pub(crate) fn refuse(
        &mut self,
        pane: &PaneId,
        message: String,
    ) -> Result<Vec<HostEvent>> {
        let Some(runtime) = self.panes.get_mut(pane) else {
            return Ok(Vec::new());
        };
        runtime.violations += 1;
        if runtime.violations >= MAX_CONSECUTIVE_VIOLATIONS {
            self.fail(pane, message)?;
            return Ok(Vec::new());
        }
        let frame = ShellFrame::ProtocolError {
            message: reason(&message)?,
        };
        self.send(pane, &frame)?;
        Ok(Vec::new())
    }

    pub(crate) fn refuse_request(
        &mut self,
        pane: &PaneId,
        id: RequestId,
        method: &MethodName,
    ) -> Result<Vec<HostEvent>> {
        let error = RpcError::new(
            ErrorCode::Unavailable,
            reason(&format!("no extension backend serves {method}"))?,
        );
        let frame = ShellFrame::Response(RpcResponse::new(id, RpcOutcome::Err(error)));
        self.send(pane, &frame)?;
        Ok(Vec::new())
    }

    pub(crate) fn report_notice(
        &mut self,
        pane: &PaneId,
        notice: IpcNotice,
    ) -> Result<Vec<HostEvent>> {
        let message = match notice {
            IpcNotice::Oversized(bytes) => {
                format!("a frame of {bytes} bytes was refused; the limit is {MAX_MESSAGE_BYTES}")
            },
            IpcNotice::Dropped => {
                let count = self.gate.take_dropped(pane);
                format!(
                    "{count} state, title or hover frames were dropped while the shell was busy"
                )
            },
        };
        tracing::warn!(%pane, %message, "a pane frame was not delivered");
        self.refuse(pane, message)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneFrame, RequestId, encode};

    use super::*;
    use crate::pane_host::reconcile::PaneDisplay;
    use crate::slot::IpcMessage;
    use crate::slot::inbox::inbox_gate::COALESCABLE_BACKLOG;
    use crate::test_support::{Fixture, fixture, frames_sent, instance, register};

    fn started() -> Fixture {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        f
    }

    fn post(
        f: &mut Fixture,
        text: String,
    ) -> Vec<HostEvent> {
        f.ipc(IpcMessage {
            pane: PaneId::new("p1").unwrap(),
            text,
        })
    }

    fn errors(f: &Fixture) -> Vec<String> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                ShellFrame::ProtocolError { message } => Some(message.to_string()),
                _ => None,
            })
            .collect()
    }

    fn json(frame: &str) -> String {
        let value: PaneFrame = serde_json::from_str(frame).unwrap();
        encode(&value).unwrap()
    }

    #[test]
    fn a_bad_frame_is_answered_and_the_pane_keeps_working() {
        let cases = [
            ("not json", "not json".to_owned()),
            (
                "an unknown kind",
                r#"{"sdk":1,"kind":"made.up"}"#.to_owned(),
            ),
            (
                "state over the limit",
                format!(
                    r#"{{"sdk":1,"kind":"pane.setState","state":{{"k":"{}"}}}}"#,
                    "x".repeat(70_000)
                ),
            ),
            (
                "an oversized frame",
                IpcNotice::Oversized(MAX_MESSAGE_BYTES + 1).encode(),
            ),
        ];
        for (name, text) in cases {
            let mut f = started();
            assert!(post(&mut f, text).is_empty(), "{name}: no events");
            assert_eq!(errors(&f).len(), 1, "{name}: the pane is told");
            assert_eq!(
                f.host.display(&PaneId::new("p1").unwrap()),
                PaneDisplay::Web,
                "{name}: the pane stays"
            );
            let title = json(r#"{"sdk":1,"kind":"pane.setTitle","title":"ok"}"#);
            assert_eq!(post(&mut f, title).len(), 1, "{name}: still served");
        }
    }

    #[test]
    fn a_run_of_bad_frames_fails_the_pane_but_a_good_frame_resets_the_run() {
        let mut f = started();
        let good = json(r#"{"sdk":1,"kind":"pane.setTitle","title":"ok"}"#);
        for _ in 0..MAX_CONSECUTIVE_VIOLATIONS - 1 {
            post(&mut f, "junk".into());
        }
        post(&mut f, good);
        for _ in 0..MAX_CONSECUTIVE_VIOLATIONS - 1 {
            post(&mut f, "junk".into());
        }
        assert_eq!(
            f.host.display(&PaneId::new("p1").unwrap()),
            PaneDisplay::Web,
            "reset by the good frame"
        );
        post(&mut f, "junk".into());
        assert!(
            matches!(
                f.host.display(&PaneId::new("p1").unwrap()),
                PaneDisplay::Error { .. }
            ),
            "failed after a run"
        );
    }

    #[test]
    fn a_request_is_answered_with_an_error_and_a_subscription_is_refused() {
        let mut f = started();
        post(
            &mut f,
            json(r#"{"sdk":1,"kind":"request","id":4,"method":"git.status","params":null}"#),
        );
        let answered = frames_sent(&f.mock).into_iter().any(|frame| {
            matches!(
                frame,
                ShellFrame::Response(response)
                    if response.id() == RequestId::new(4)
                        && matches!(response.outcome(), RpcOutcome::Err(e) if e.code() == ErrorCode::Unavailable)
            )
        });
        assert!(answered, "the promise rejects");
        post(
            &mut f,
            json(r#"{"sdk":1,"kind":"subscribe","id":5,"event":"git.changed"}"#),
        );
        assert_eq!(errors(&f).len(), 1, "the subscription is refused out loud");
        assert!(
            post(&mut f, json(r#"{"sdk":1,"kind":"unsubscribe","id":5}"#)).is_empty(),
            "leaving is always fine"
        );
    }

    #[test]
    fn dropped_frames_are_counted_and_reported_to_the_pane() {
        let mut f = started();
        let title = json(r#"{"sdk":1,"kind":"pane.setTitle","title":"ok"}"#);
        let pane = PaneId::new("p1").unwrap();
        for _ in 0..COALESCABLE_BACKLOG + 3 {
            f.host.gate.admit(pane.clone(), &title);
        }
        let inbox = f.host.inbox();
        while let Ok(message) = inbox.try_recv() {
            f.ipc(message);
        }
        let told = errors(&f);
        assert!(told.iter().any(|m| m.starts_with("3 state")), "{told:?}");
    }
}
