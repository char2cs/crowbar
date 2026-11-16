//! What a session tells the terminal when its stream is lost, and how it waits to reattach.

use std::pin::pin;

use async_channel::Receiver;
use futures_util::StreamExt;
use futures_util::future::ready;
use terminal::{ClientMessage, DaemonFrame};
use tokio::time::sleep;

use crate::ends::Ends;
use crate::reconnect::Reconnect;

const WARNING_COLOR: &str = "\x1b[33m";
const RESET: &str = "\x1b[0m";

pub(crate) async fn recover(
    ends: &Ends,
    reconnect: &mut Reconnect,
    reason: &str,
) -> bool {
    if ends.messages.is_closed() {
        return false;
    }
    let Some(delay) = reconnect.next_delay() else {
        notice(
            ends,
            &format!(
                "{WARNING_COLOR}terminal disconnected ({reason}); press a key to reconnect{RESET}"
            ),
        )
        .await;
        let pressed = next_input(&ends.messages).await;
        reconnect.connected();
        return pressed;
    };
    notice(
        ends,
        &format!("{WARNING_COLOR}terminal disconnected ({reason}); reconnecting{RESET}"),
    )
    .await;
    sleep(delay).await;
    true
}

async fn next_input(messages: &Receiver<ClientMessage>) -> bool {
    let inputs = messages
        .clone()
        .filter(|message| ready(matches!(message, ClientMessage::Input(_))));
    pin!(inputs).next().await.is_some()
}

pub(crate) async fn notice(
    ends: &Ends,
    text: &str,
) {
    let frame = DaemonFrame::Output(format!("\r\n{text}\r\n").into_bytes());
    if ends.frames.send(frame).await.is_err() {
        tracing::debug!("the terminal closed before its notice");
    }
}

#[cfg(test)]
mod tests {
    use async_channel::unbounded;

    use super::*;

    const GIVE_UP_AFTER: usize = 8;

    fn exhausted() -> Reconnect {
        let mut reconnect = Reconnect::default();
        for _ in 0..GIVE_UP_AFTER {
            reconnect.next_delay();
        }
        reconnect
    }

    async fn notice_text(frames: &Receiver<DaemonFrame>) -> String {
        let frame = frames.recv().await.unwrap();
        let DaemonFrame::Output(bytes) = frame else {
            panic!("expected a notice");
        };
        String::from_utf8(bytes).unwrap()
    }

    #[tokio::test]
    async fn a_closed_view_is_not_reconnected() {
        let (frames, _view_frames) = unbounded();
        let (view_messages, messages) = unbounded::<ClientMessage>();
        drop(view_messages);
        let ends = Ends { frames, messages };
        assert!(!recover(&ends, &mut Reconnect::default(), "gone").await);
    }

    #[tokio::test]
    async fn after_giving_up_a_key_press_reconnects() {
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let ends = Ends { frames, messages };
        let typed = ClientMessage::Input("x".into());
        view_messages.send(typed).await.unwrap();
        assert!(recover(&ends, &mut exhausted(), "lost").await);
        let text = notice_text(&view_frames).await;
        assert!(text.contains("press a key to reconnect"), "{text}");
    }
}
