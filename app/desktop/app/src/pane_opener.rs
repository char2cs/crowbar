use model::{Command, Layout, PaneInstance};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use crate::error::Result;
use crate::serials::Serials;

/// The commands that bring the pane `type_id` with `params` to the front: an existing pane is
/// focused, otherwise a new one opens in the active tab (or in a new tab when there is none).
pub(crate) fn open_commands(
    layout: &Layout,
    pane: (PaneTypeId, Params),
    serials: &Serials,
) -> Result<Vec<Command>> {
    let (type_id, params) = pane;
    if let Some(existing) = layout.find_pane(&type_id, &params) {
        return Ok(focus_commands(layout, &existing.pane_id));
    }
    let instance = PaneInstance {
        pane_id: PaneId::new(serials.next_pane())?,
        type_id,
        params,
        state: PaneState::default(),
    };
    let Some(tab) = layout.active_tab() else {
        let id = serials.next_tab();
        return Ok(vec![
            Command::AddTab {
                id,
                title: String::new(),
            },
            Command::OpenPane {
                tab: id,
                instance,
                placement: None,
            },
        ]);
    };
    Ok(vec![Command::OpenPane {
        tab: tab.id(),
        instance,
        placement: None,
    }])
}

fn focus_commands(layout: &Layout, pane: &PaneId) -> Vec<Command> {
    let tab = layout.tab_of(pane).map(model::Tab::id);
    let mut commands: Vec<Command> = tab
        .map(|tab| Command::ActivateTab { tab })
        .into_iter()
        .collect();
    commands.push(Command::FocusPane { pane: pane.clone() });
    commands
}

#[cfg(test)]
#[path = "pane_opener_tests.rs"]
mod tests;
