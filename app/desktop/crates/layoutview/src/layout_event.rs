use layout::{Command, Placement, TabId};
use protocol::PaneId;

use ideshell::PathAction;

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum LayoutEvent {
    Apply(Command),
    OpenChat {
        chat: String,
        tab: TabId,
        placement: Placement,
    },
    OpenFile {
        path: String,
        onto: PaneId,
    },
    StageChat {
        chat: String,
    },
    DropTab {
        tab: TabId,
        onto: TabId,
        placement: Placement,
    },
    StripFocus {
        pane: PaneId,
    },
    TabPath {
        action: PathAction,
        path: String,
    },
    RenameTab {
        pane: PaneId,
        current: String,
    },
    ReloadTab {
        pane: PaneId,
    },
    Hovered,
    Announce(String),
    NewTab {
        onto: PaneId,
    },
    ReviewBranch {
        onto: PaneId,
    },
}

impl LayoutEvent {
    pub(crate) fn tab_path(
        action: PathAction,
        path: &str,
    ) -> Self {
        Self::TabPath {
            action,
            path: path.to_owned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_path_event_names_the_action_and_the_file() {
        let event = LayoutEvent::tab_path(PathAction::Reveal, "src/main.rs");
        let expected = LayoutEvent::TabPath {
            action: PathAction::Reveal,
            path: "src/main.rs".into(),
        };
        assert_eq!(event, expected);
    }
}
