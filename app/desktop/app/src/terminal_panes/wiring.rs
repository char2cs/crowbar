use gpui::App;
use protocol::{HostId, PaneId};
use ptysession::{Binding, Connection, Ends, Report, TerminalSessions};
use terminal::DaemonFrame;

use super::TerminalPanes;
use crate::shell_request::ShellRequest;
use crate::terminal_owner::TerminalOwner;

const NO_CHAT: &str = "Open a chat first: a terminal runs in a chat's workspace.\r\n";

impl TerminalPanes {
    pub(super) fn connect(
        &self,
        owner: Option<&TerminalOwner>,
        connection: Connection,
    ) {
        let base = owner.map(TerminalOwner::base);
        let (Some(owner), Some(Ok(base))) = (owner, base) else {
            notify(&connection.ends, NO_CHAT);
            return;
        };
        let binding = self
            .ids
            .saved_session(&connection.pane)
            .map_or(Binding::Create, Binding::Saved);
        self.sessions_of(owner).open(base, binding, connection);
    }

    fn sessions_of(
        &self,
        owner: &TerminalOwner,
    ) -> TerminalSessions {
        let TerminalOwner::Chat(chat) = owner else {
            return self.sessions.clone();
        };
        let host = self
            .hosts
            .host(chat)
            .and_then(|host| HostId::new(host).ok());
        host.map_or_else(|| self.sessions.clone(), |host| self.sessions.on(host))
    }

    pub(super) fn report_into_shell(
        requests: async_channel::Sender<ShellRequest>,
        pane: PaneId,
        cx: &mut App,
    ) -> async_channel::Sender<Report> {
        let (reports, heard) = async_channel::unbounded();
        cx.spawn(async move |_| relay(heard, requests, pane).await)
            .detach();
        reports
    }

    pub(super) fn forward(
        requests: &async_channel::Sender<ShellRequest>,
        pane: &PaneId,
        event: &terminalview::Event,
    ) {
        let request = ShellRequest::Terminal {
            pane: pane.clone(),
            event: event.clone(),
        };
        if requests.try_send(request).is_err() {
            tracing::debug!("the shell is closing; a terminal's event goes unseen");
        }
    }
}

async fn relay(
    heard: async_channel::Receiver<Report>,
    requests: async_channel::Sender<ShellRequest>,
    pane: PaneId,
) {
    while let Ok(report) = heard.recv().await
        && requests.send(session_request(&pane, report)).await.is_ok()
    {}
}

fn session_request(
    pane: &PaneId,
    report: Report,
) -> ShellRequest {
    let pane = pane.clone();
    ShellRequest::TerminalSession { pane, report }
}

fn notify(
    ends: &Ends,
    text: &str,
) {
    let frame = DaemonFrame::Output(text.as_bytes().to_vec());
    if ends.frames.try_send(frame).is_err() {
        tracing::debug!("the terminal closed before its notice");
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::{idle_terminals_into, terminal_layout};

    #[gpui::test]
    fn what_a_terminal_tells_its_host_reaches_the_shell(cx: &mut TestAppContext) {
        cx.executor().allow_parking();
        cx.update(|cx| ui::init(cx).unwrap());
        let ids = TerminalIds::default();
        ids.follow(&terminal_layout(Some("c1")));
        let (requests, queue) = async_channel::unbounded();
        let panes = idle_terminals_into(ids, requests);
        let (_root, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let pane = PaneId::new("t1").unwrap();
        let view = cx
            .update(|window, cx| panes.view(&pane, window, cx))
            .unwrap();
        let event = terminalview::Event::OpenFile {
            path: "src/a.rs".into(),
            line: None,
        };
        let sent = event.clone();
        view.update(cx, |_, cx| cx.emit(sent));
        cx.run_until_parked();
        assert_eq!(queue.try_recv(), Ok(ShellRequest::Terminal { pane, event }));
    }
}
