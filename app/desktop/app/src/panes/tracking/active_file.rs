use layout::Layout;

use crate::panes::ide::tab_looks;
use crate::panes::tracking::group_focus::focused_group;

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-sync.ts
pub(crate) fn active_file(
    layout: &Layout,
    space: &str,
) -> Option<String> {
    let group = focused_group(layout.showing(space)?)?;
    let instance = layout.pane(group.active_tab()?)?;
    tab_looks::file_path(instance)
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;

    fn instance(
        (name, kind): (&str, &str),
        state: serde_json::Value,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new(kind).unwrap(),
            params: Params::default().with("id", name).unwrap(),
            state: PaneState::new(state).unwrap(),
        }
    }

    type Opened<'a> = ((&'a str, &'a str), serde_json::Value);

    fn layout_of(panes: &[Opened<'_>]) -> Layout {
        let mut layout = Layout::default();
        let tab = TabId(1);
        let add = Command::AddTab {
            id: tab,
            title: String::new(),
            space: layout::SpaceId::default(),
        };
        layout.apply(add).unwrap();
        for (pane, state) in panes {
            layout
                .apply(open(tab, instance(*pane, state.clone())))
                .unwrap();
        }
        layout
    }

    fn open(
        tab: TabId,
        instance: PaneInstance,
    ) -> Command {
        Command::OpenPane {
            tab,
            instance,
            placement: None,
        }
    }

    #[test]
    fn the_active_editor_tab_of_the_focused_group_names_the_file() {
        let chat = (("c1", "core.chat"), json!({}));
        let first = (("e1", "core.editor"), json!({"path": "src/a.rs"}));
        let second = (("e2", "core.editor"), json!({"path": "src/b.rs"}));
        let terminal = (("t1", "core.terminal"), json!({}));
        let cases = [
            (vec![], None),
            (vec![chat.clone()], None),
            (vec![chat.clone(), first.clone()], Some("src/a.rs")),
            (vec![chat.clone(), first, second.clone()], Some("src/b.rs")),
            (vec![chat, second, terminal], None),
        ];
        for (panes, want) in cases {
            let layout = layout_of(&panes);
            assert_eq!(active_file(&layout, "").as_deref(), want, "{panes:?}");
        }
    }
}
