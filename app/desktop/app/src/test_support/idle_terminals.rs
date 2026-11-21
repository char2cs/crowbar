use std::rc::Rc;
use std::sync::{Arc, OnceLock};

use protocol::HostId;
use ptysession::TerminalSessions;
use tokio::runtime::Runtime;

use super::NoDaemon;
use crate::chat_terminals::ChatTerminals;
use crate::shell_request::ShellRequest;
use crate::terminal_ids::TerminalIds;
use crate::terminal_panes::TerminalPanes;

static RUNTIME: OnceLock<Runtime> = OnceLock::new();

/// Terminal panes whose sessions can never connect.
pub(crate) fn idle_terminals(ids: TerminalIds) -> TerminalPanes {
    idle_terminals_for(ids, no_chats())
}

/// Idle terminal panes that tell the shell through `requests`.
pub(crate) fn idle_terminals_into(
    ids: TerminalIds,
    requests: async_channel::Sender<ShellRequest>,
) -> TerminalPanes {
    TerminalPanes::new(ids, (idle_sessions(), no_chats()), requests)
}

pub(crate) fn idle_terminals_for(
    ids: TerminalIds,
    chats: ChatTerminals,
) -> TerminalPanes {
    TerminalPanes::new(ids, (idle_sessions(), chats), async_channel::unbounded().0)
}

fn no_chats() -> ChatTerminals {
    ChatTerminals::new(Rc::new(|_| None), (Rc::new(|_| {}), Rc::new(|_| None)))
}

fn idle_sessions() -> TerminalSessions {
    let runtime = RUNTIME.get_or_init(|| Runtime::new().unwrap());
    let host = HostId::new("local").unwrap();
    let transport = Arc::new(NoDaemon::timing_out());
    TerminalSessions::new(transport, host, runtime.handle().clone(), "crowbar://local")
}
