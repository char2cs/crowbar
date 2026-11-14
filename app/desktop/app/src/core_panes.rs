use model::{Command, PaneInstance, TabId};
use pane_host::{Multiplicity, PaneLifecycle, PaneRegistry, PaneType};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

pub(crate) const FIRST_TAB: TabId = TabId(1);

pub(crate) fn register_core_panes(
    registry: &mut PaneRegistry,
    chat_entry: &str,
) -> anyhow::Result<()> {
    registry.register(PaneType {
        id: PaneTypeId::new("core.chat")?,
        title: "Chat".into(),
        entry: chat_entry.into(),
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::PerParams,
    })?;
    Ok(())
}

pub(crate) fn first_commands() -> protocol::Result<Vec<Command>> {
    Ok(vec![
        Command::AddTab {
            id: FIRST_TAB,
            title: "Chat".into(),
        },
        Command::OpenPane {
            tab: FIRST_TAB,
            instance: PaneInstance {
                pane_id: PaneId::new("chat-1")?,
                type_id: PaneTypeId::new("core.chat")?,
                params: Params::default(),
                state: PaneState::default(),
            },
            placement: None,
        },
    ])
}

#[cfg(test)]
#[path = "core_panes_tests.rs"]
mod tests;
