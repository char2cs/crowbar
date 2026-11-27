use std::rc::Rc;
use std::sync::{Arc, OnceLock};

use protocol::HostId;
use ptysession::TerminalSessions;
use tokio::runtime::{Builder, Runtime};

use super::NoDaemon;
use crate::panes::terminals::chat_hosts::ChatHosts;
use crate::panes::terminals::terminal_ids::TerminalIds;
use crate::panes::terminals::terminal_panes::TerminalPanes;
use crate::shell::inbox::shell_request::ShellRequest;

static UNDRIVEN: OnceLock<Runtime> = OnceLock::new();

pub(crate) fn idle_terminals(ids: TerminalIds) -> TerminalPanes {
    TerminalPanes::new(
        ids,
        (idle_sessions(), no_chats()),
        async_channel::unbounded().0,
    )
}

pub(crate) fn idle_terminals_into(
    ids: TerminalIds,
    requests: async_channel::Sender<ShellRequest>,
) -> TerminalPanes {
    TerminalPanes::new(ids, (idle_sessions(), no_chats()), requests)
}

pub(crate) fn heard_terminals(
    ids: TerminalIds,
    heard: async_channel::Sender<String>,
) -> (TerminalPanes, Runtime) {
    let runtime = current_thread();
    let sessions = sessions_on(NoDaemon::telling(heard), &runtime);
    let panes = TerminalPanes::new(ids, (sessions, no_chats()), async_channel::unbounded().0);
    (panes, runtime)
}

fn no_chats() -> ChatHosts {
    ChatHosts::new((Rc::new(|_| None), Rc::new(|_| None)))
}

fn idle_sessions() -> TerminalSessions {
    sessions_on(NoDaemon::timing_out(), UNDRIVEN.get_or_init(current_thread))
}

fn current_thread() -> Runtime {
    Builder::new_current_thread().enable_all().build().unwrap()
}

fn sessions_on(
    daemon: NoDaemon,
    runtime: &Runtime,
) -> TerminalSessions {
    let host = HostId::new("local").unwrap();
    TerminalSessions::new(
        Arc::new(daemon),
        host,
        runtime.handle().clone(),
        "crowbar://local",
    )
}
