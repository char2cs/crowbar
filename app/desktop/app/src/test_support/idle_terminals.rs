use std::rc::Rc;
use std::sync::{Arc, OnceLock};

use protocol::HostId;
use terminal_session::TerminalSessions;
use tokio::runtime::Runtime;

use super::NoDaemon;
use crate::chat_terminals::ChatTerminals;
use crate::terminal_ids::TerminalIds;
use crate::terminal_panes::TerminalPanes;

static RUNTIME: OnceLock<Runtime> = OnceLock::new();

/// Terminal panes whose sessions can never connect.
pub(crate) fn idle_terminals(ids: TerminalIds) -> TerminalPanes {
    let runtime = RUNTIME.get_or_init(|| Runtime::new().unwrap());
    let host = HostId::new("local").unwrap();
    let transport = Arc::new(NoDaemon::timing_out());
    let sessions =
        TerminalSessions::new(transport, host, runtime.handle().clone(), "crowbar://local");
    TerminalPanes::new(
        ids,
        (
            sessions,
            ChatTerminals::new(Rc::new(|_| None), Rc::new(|_| {})),
        ),
    )
}
