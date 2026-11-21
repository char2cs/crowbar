use std::rc::Rc;
use std::sync::{Arc, OnceLock, mpsc};

use protocol::HostId;
use ptysession::TerminalSessions;
use tokio::runtime::Runtime;

use super::NoDaemon;
use crate::chat_hosts::ChatHosts;
use crate::shell_request::ShellRequest;
use crate::terminal_ids::TerminalIds;
use crate::terminal_panes::TerminalPanes;

static RUNTIME: OnceLock<Runtime> = OnceLock::new();

/// Terminal panes whose sessions can never connect.
pub(crate) fn idle_terminals(ids: TerminalIds) -> TerminalPanes {
    TerminalPanes::new(
        ids,
        (idle_sessions(), no_chats()),
        async_channel::unbounded().0,
    )
}

/// Idle terminal panes that tell the shell through `requests`.
pub(crate) fn idle_terminals_into(
    ids: TerminalIds,
    requests: async_channel::Sender<ShellRequest>,
) -> TerminalPanes {
    TerminalPanes::new(ids, (idle_sessions(), no_chats()), requests)
}

/// Terminal panes telling `heard` each request their sessions make of the daemon.
pub(crate) fn heard_terminals(
    ids: TerminalIds,
    heard: mpsc::Sender<String>,
) -> TerminalPanes {
    let sessions = sessions_on(NoDaemon::telling(heard));
    TerminalPanes::new(ids, (sessions, no_chats()), async_channel::unbounded().0)
}

fn no_chats() -> ChatHosts {
    ChatHosts::new(Rc::new(|_| None))
}

fn idle_sessions() -> TerminalSessions {
    sessions_on(NoDaemon::timing_out())
}

fn sessions_on(daemon: NoDaemon) -> TerminalSessions {
    let runtime = RUNTIME.get_or_init(|| Runtime::new().unwrap());
    let host = HostId::new("local").unwrap();
    TerminalSessions::new(
        Arc::new(daemon),
        host,
        runtime.handle().clone(),
        "crowbar://local",
    )
}
