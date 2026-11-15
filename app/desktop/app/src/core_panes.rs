use pane_host::{Multiplicity, PaneLifecycle, PaneRegistry, PaneType};
use protocol::{PaneTypeId, Params};

use crate::error::Result;

/// The pane type of a chat thread.
pub(crate) fn chat_type() -> Result<PaneTypeId> {
    Ok(PaneTypeId::new("core.chat")?)
}

/// The params that identify the chat of thread `thread`.
pub(crate) fn chat_params(thread: &str) -> Result<Params> {
    Ok(Params::default().with("threadId", thread)?)
}

/// Adds the built-in pane types to `registry`; the chat page is `chat_entry` in the core bundle.
pub(crate) fn register_core_panes(registry: &mut PaneRegistry, chat_entry: &str) -> Result<()> {
    registry.register(PaneType {
        id: chat_type()?,
        title: "Chat".into(),
        entry: chat_entry.into(),
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::PerParams,
    })?;
    Ok(())
}

#[cfg(test)]
#[path = "core_panes_tests.rs"]
mod tests;
