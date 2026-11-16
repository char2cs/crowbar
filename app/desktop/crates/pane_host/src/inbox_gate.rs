use std::collections::BTreeMap;
use std::sync::Mutex;

use protocol::PaneId;

use crate::frame_kind::FrameKind;
use crate::ipc_message::IpcMessage;
use crate::ipc_notice::IpcNotice;

pub(crate) const COALESCABLE_BACKLOG: usize = 256;
pub(crate) const MAX_MESSAGE_BYTES: usize = 1 << 20;
const COALESCABLE_KINDS: [&str; 3] = ["pane.setState", "pane.setTitle", "shell.hover"];

pub(crate) struct InboxGate {
    sink: async_channel::Sender<IpcMessage>,
    dropped: Mutex<BTreeMap<PaneId, u64>>,
}

impl InboxGate {
    pub(crate) fn new(sink: async_channel::Sender<IpcMessage>) -> Self {
        Self {
            sink,
            dropped: Mutex::default(),
        }
    }

    pub(crate) fn sink(&self) -> async_channel::Sender<IpcMessage> {
        self.sink.clone()
    }

    pub(crate) fn admit(
        &self,
        pane: PaneId,
        text: &str,
    ) {
        if text.len() > MAX_MESSAGE_BYTES {
            return self.push(pane, IpcNotice::Oversized(text.len()).encode());
        }
        if self.sink.len() >= COALESCABLE_BACKLOG && is_coalescable(text) {
            return self.drop_frame(pane);
        }
        self.push(pane, text.to_owned());
    }

    pub(crate) fn take_dropped(
        &self,
        pane: &PaneId,
    ) -> u64 {
        self.dropped
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(pane)
            .unwrap_or(0)
    }

    fn drop_frame(
        &self,
        pane: PaneId,
    ) {
        let first = {
            let mut dropped = self
                .dropped
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            let count = dropped.entry(pane.clone()).or_insert(0);
            *count += 1;
            *count == 1
        };
        if first {
            self.push(pane, IpcNotice::Dropped.encode());
        }
    }

    fn push(
        &self,
        pane: PaneId,
        text: String,
    ) {
        if self.sink.try_send(IpcMessage { pane, text }).is_err() {
            tracing::warn!("the pane host inbox is closed; a pane message was lost");
        }
    }
}

fn is_coalescable(text: &str) -> bool {
    serde_json::from_str::<FrameKind<'_>>(text)
        .is_ok_and(|frame| COALESCABLE_KINDS.contains(&frame.kind))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn gate() -> (InboxGate, async_channel::Receiver<IpcMessage>) {
        let (sink, inbox) = async_channel::unbounded();
        (InboxGate::new(sink), inbox)
    }

    fn pane() -> PaneId {
        PaneId::new("p1").unwrap()
    }

    const REGISTER: &str =
        r#"{"sdk":1,"kind":"pane.register","typeId":"core.chat","sdkVersion":"1.0"}"#;
    const TITLE: &str = r#"{"sdk":1,"kind":"pane.setTitle","title":"x"}"#;

    #[test]
    fn a_full_backlog_drops_only_latest_wins_frames_and_never_the_register() {
        let (gate, inbox) = gate();
        for _ in 0..COALESCABLE_BACKLOG {
            gate.admit(pane(), TITLE);
        }
        assert_eq!(inbox.len(), COALESCABLE_BACKLOG, "backlog filled");
        gate.admit(pane(), TITLE);
        gate.admit(pane(), TITLE);
        gate.admit(pane(), REGISTER);
        let queued: Vec<String> = std::iter::from_fn(|| inbox.try_recv().ok())
            .skip(COALESCABLE_BACKLOG)
            .map(|message| message.text)
            .collect();
        assert_eq!(
            queued,
            [IpcNotice::Dropped.encode(), REGISTER.to_owned()],
            "one notice for the burst, then the register"
        );
        assert_eq!(gate.take_dropped(&pane()), 2, "counted");
        assert_eq!(gate.take_dropped(&pane()), 0, "taken once");
    }

    #[test]
    fn an_oversized_frame_is_replaced_by_a_notice_with_its_size() {
        let (gate, inbox) = gate();
        gate.admit(pane(), &"x".repeat(MAX_MESSAGE_BYTES + 1));
        assert_eq!(
            inbox.try_recv().unwrap().text,
            IpcNotice::Oversized(MAX_MESSAGE_BYTES + 1).encode(),
            "notice"
        );
    }
}
