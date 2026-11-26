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
    use layout::PaneInstance;
    use protocol::{PaneState, Params};
    use serde_json::json;

    use super::*;
    use crate::test_support::{one_tab, pane};

    fn instance(
        (name, kind): (&str, &str),
        state: serde_json::Value,
    ) -> PaneInstance {
        PaneInstance {
            params: Params::default().with("id", name).unwrap(),
            state: PaneState::new(state).unwrap(),
            ..pane(name, kind)
        }
    }

    type Opened<'a> = ((&'a str, &'a str), serde_json::Value);

    fn layout_of(panes: &[Opened<'_>]) -> Layout {
        one_tab(
            panes
                .iter()
                .map(|(pane, state)| instance(*pane, state.clone())),
        )
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
