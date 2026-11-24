use layout::{Command, Layout, PaneInstance};

use crate::close_guard::closing_panes;
use crate::pane_spec::PaneSpec;

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/editor/config/constants.ts
const REMEMBERED: usize = 10;
const PATH: &str = "path";
const PREVIEW: &str = "preview";

#[derive(Debug, Default)]
pub(crate) struct ClosedPanes {
    specs: Vec<PaneSpec>,
}

impl ClosedPanes {
    pub(crate) fn closing(
        layout: &Layout,
        command: &Command,
    ) -> Vec<PaneSpec> {
        closing_panes(layout, command)
            .iter()
            .filter_map(|pane| layout.pane(pane))
            .filter(|instance| is_file_tab(instance))
            .map(PaneSpec::of)
            .collect()
    }

    pub(crate) fn remember(
        &mut self,
        closed: Vec<PaneSpec>,
    ) {
        self.specs.extend(closed);
        let excess = self.specs.len().saturating_sub(REMEMBERED);
        self.specs.drain(..excess);
    }

    pub(crate) fn take_last(&mut self) -> Option<PaneSpec> {
        self.specs.pop()
    }
}

fn is_file_tab(instance: &PaneInstance) -> bool {
    let state = instance.state.as_value();
    let named = state
        .get(PATH)
        .and_then(serde_json::Value::as_str)
        .is_some_and(|path| !path.is_empty());
    crate::tab_looks::is_file(instance) && named && state.get(PREVIEW).is_none()
}

#[cfg(test)]
mod tests {
    use layout::TabId;
    use protocol::PaneId;

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    fn open_pane(
        layout: &mut Layout,
        instance: PaneInstance,
    ) {
        let command = Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        };
        layout.apply(command).unwrap();
    }

    fn file(id: &str) -> PaneInstance {
        let spec = PaneSpec::editor("c1", &format!("src/{id}.ts")).unwrap();
        spec.instance(PaneId::new(id).unwrap())
    }

    fn layout_of(instances: Vec<PaneInstance>) -> Layout {
        let mut layout = Layout::default();
        let add = Command::AddTab {
            id: TabId(1),
            title: String::new(),
        };
        layout.apply(add).unwrap();
        for instance in instances {
            open_pane(&mut layout, instance);
        }
        layout
    }

    fn mixed() -> Layout {
        let chat = PaneInstance {
            pane_id: PaneId::new("chat").unwrap(),
            type_id: chat_type().unwrap(),
            params: chat_params("c1").unwrap(),
            state: protocol::PaneState::default(),
        };
        let untitled = PaneSpec::untitled("c1", ("1", "Untitled")).unwrap();
        let preview = PaneSpec::preview(
            "c1",
            (
                &protocol::DocumentPath::new("src/b.md").unwrap(),
                editor::Preview::Markdown,
            ),
        );
        layout_of(vec![
            chat,
            file("a"),
            untitled.instance(PaneId::new("u").unwrap()),
            preview.unwrap().instance(PaneId::new("p").unwrap()),
            file("b"),
        ])
    }

    fn paths(specs: &[PaneSpec]) -> Vec<String> {
        specs
            .iter()
            .filter_map(|spec| spec.state.as_value().get(PATH)?.as_str().map(str::to_owned))
            .collect()
    }

    #[test]
    fn closing_a_tab_remembers_only_its_file_tabs() {
        let closing = ClosedPanes::closing(&mixed(), &Command::CloseTab { tab: TabId(1) });
        assert_eq!(paths(&closing), ["src/a.ts", "src/b.ts"]);
    }

    #[test]
    fn closing_names_the_file_panes_the_command_closes() {
        let layout = mixed();
        let one = |id: &str| PaneId::new(id).unwrap();
        let cases = [
            (Command::ClosePane { pane: one("a") }, vec!["src/a.ts"]),
            (Command::ClosePane { pane: one("chat") }, vec![]),
            (
                Command::ClosePanes {
                    panes: vec![one("u"), one("b")],
                },
                vec!["src/b.ts"],
            ),
            (Command::FocusPane { pane: one("a") }, vec![]),
        ];
        for (command, expected) in cases {
            assert_eq!(paths(&ClosedPanes::closing(&layout, &command)), expected);
        }
    }

    #[test]
    fn the_last_closed_comes_back_first_and_only_the_newest_ten_are_kept() {
        let layout = layout_of(vec![file("a"), file("b")]);
        let both = ClosedPanes::closing(&layout, &Command::CloseTab { tab: TabId(1) });
        let mut closed = ClosedPanes::default();
        closed.remember(both.clone());
        assert_eq!(
            paths(&[closed.take_last().unwrap()]),
            ["src/b.ts"],
            "newest"
        );
        assert!(closed.take_last().is_some(), "then the other");
        assert!(closed.take_last().is_none(), "then nothing");
        for _ in 0..REMEMBERED {
            closed.remember(both.clone());
        }
        assert_eq!(closed.specs.len(), 10, "capped as develop");
    }
}
