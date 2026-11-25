//! What a session tells the terminal when its stream is lost, and how it waits to reattach.

use std::pin::pin;
use std::time::Duration;

use async_channel::Receiver;
use futures_util::StreamExt;
use futures_util::future::ready;
use terminal::{ClientMessage, DaemonFrame};
use tokio::time::sleep;

use crate::ends::Ends;
use crate::stream::reconnect::Reconnect;

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
        return wait_for_key(ends, reconnect, reason).await;
    };
    report_reattaching(reason, delay);
    sleep(delay).await;
    true
}

async fn wait_for_key(
    ends: &Ends,
    reconnect: &mut Reconnect,
    reason: &str,
) -> bool {
    notice(
        ends,
        &format!(
            "{WARNING_COLOR}terminal disconnected ({reason}); press a key to reconnect{RESET}"
        ),
    )
    .await;
    let pressed = next_input(&ends.messages).await;
    reconnect.connected();
    pressed
}

fn report_reattaching(
    reason: &str,
    delay: Duration,
) {
    tracing::debug!(%reason, ?delay, "reattaching the terminal");
}

async fn next_input(messages: &Receiver<ClientMessage>) -> bool {
    let inputs = messages
        .clone()
        .filter(|message| ready(matches!(message, ClientMessage::Input(_))));
    pin!(inputs).next().await.is_some()
}

async fn notice(
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

    #[derive(Clone, Copy)]
    enum Before {
        ViewClosed,
        FirstDrop,
        GaveUpThenKey,
    }

    struct Case {
        before: Before,
        reconnects: bool,
        shown: &'static str,
    }

    const CASES: [Case; 3] = [
        Case {
            before: Before::ViewClosed,
            reconnects: false,
            shown: "",
        },
        Case {
            before: Before::FirstDrop,
            reconnects: true,
            shown: "",
        },
        Case {
            before: Before::GaveUpThenKey,
            reconnects: true,
            shown: "press a key to reconnect",
        },
    ];

    async fn recover_after(before: Before) -> (bool, String) {
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let mut reconnect = Reconnect::default();
        match before {
            Before::ViewClosed => drop(view_messages),
            Before::FirstDrop => {},
            Before::GaveUpThenKey => {
                reconnect = exhausted();
                view_messages
                    .send(ClientMessage::Input("x".into()))
                    .await
                    .unwrap();
            },
        }
        let ends = Ends { frames, messages };
        let reconnects = recover(&ends, &mut reconnect, "lost").await;
        drop(ends);
        (reconnects, shown(&view_frames))
    }

    fn shown(frames: &Receiver<DaemonFrame>) -> String {
        let mut text = String::new();
        while let Ok(DaemonFrame::Output(bytes)) = frames.try_recv() {
            text.push_str(&String::from_utf8_lossy(&bytes));
        }
        text
    }

    #[tokio::test(start_paused = true)]
    async fn a_lost_stream_reattaches_quietly_until_it_gives_up_or_the_view_closes() {
        check_all().await;
    }

    async fn check_all() {
        for case in CASES {
            let (reconnects, shown) = recover_after(case.before).await;
            assert_eq!(reconnects, case.reconnects);
            let same_silence = shown.is_empty() == case.shown.is_empty();
            assert!(same_silence && shown.contains(case.shown), "{shown:?}");
        }
    }
}
