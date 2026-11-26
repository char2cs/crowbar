use layout::{Command, Layout};
use protocol::PaneId;

use crate::panes::kinds::pane_spec::PaneSpec;
use crate::shell::shell_state::ShellState;

impl ShellState {
    pub(crate) fn preview_spec(
        &mut self,
        spec: PaneSpec,
    ) {
        match self.spec_commands(spec) {
            Ok(commands) => self.apply(previewing(commands)),
            Err(error) => tracing::warn!(%error, "could not preview the pane"),
        }
    }

    pub(crate) fn mark_dirty(
        &mut self,
        pane: PaneId,
    ) {
        self.keep_if_preview(&pane);
        self.dirty.mark(pane);
    }

    pub(crate) fn clear_dirty(
        &mut self,
        pane: &PaneId,
    ) {
        self.dirty.clear(pane);
    }

    pub(crate) fn keep_if_preview(
        &mut self,
        pane: &PaneId,
    ) {
        if !is_preview(self.history.layout(), pane) {
            return;
        }
        self.run(vec![Command::KeepPane { pane: pane.clone() }]);
    }
}

pub(crate) fn keeping(
    layout: &Layout,
    commands: Vec<Command>,
) -> Vec<Command> {
    let kept: Vec<Command> = commands
        .iter()
        .filter_map(focused_pane)
        .filter(|pane| is_preview(layout, pane))
        .map(|pane| Command::KeepPane { pane })
        .collect();
    commands.into_iter().chain(kept).collect()
}

fn is_preview(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let group = layout.tab_of(pane).and_then(|tab| tab.group_of(pane));
    group.and_then(layout::PaneGroup::preview) == Some(pane)
}

fn focused_pane(command: &Command) -> Option<PaneId> {
    match command {
        Command::FocusPane { pane } => Some(pane.clone()),
        _ => None,
    }
}

pub(crate) fn previewing(commands: Vec<Command>) -> Vec<Command> {
    let fresh: Vec<PaneId> = commands.iter().filter_map(opened_pane).collect();
    let previews = fresh.into_iter().map(|pane| Command::PreviewPane { pane });
    commands.into_iter().chain(previews).collect()
}

fn opened_pane(command: &Command) -> Option<PaneId> {
    match command {
        Command::OpenPane { instance, .. } | Command::OpenPaneInGroup { instance, .. } => {
            Some(instance.pane_id.clone())
        },
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneState;

    use super::*;
    use crate::test_support::{open_chat, resync, shell_rig};

    fn preview_of(shell: &ShellState) -> Option<String> {
        let tab = shell.showing()?;
        let chat = tab.pane_ids().into_iter().next()?;
        let preview = tab.group_of(&chat)?.preview()?;
        let state: &PaneState = &shell.layout().pane(preview)?.state;
        let state = state.as_value();
        state.get("path")?.as_str().map(ToOwned::to_owned)
    }

    enum Step {
        Preview(&'static str),
        Open(&'static str),
    }

    type Case = (&'static str, &'static [Step], Option<&'static str>, usize);

    const CASES: &[Case] = &[
        (
            "a file opened from the tree is the preview",
            &[Step::Preview("a.rs")],
            Some("a.rs"),
            2,
        ),
        (
            "the next one takes it over",
            &[Step::Preview("a.rs"), Step::Preview("b.rs")],
            Some("b.rs"),
            3,
        ),
        (
            "previewing an open file again leaves the preview where it is",
            &[
                Step::Preview("a.rs"),
                Step::Preview("b.rs"),
                Step::Preview("a.rs"),
            ],
            Some("b.rs"),
            3,
        ),
        (
            "a kept open of the preview, as a double click, keeps it",
            &[
                Step::Preview("a.rs"),
                Step::Preview("b.rs"),
                Step::Open("b.rs"),
            ],
            None,
            3,
        ),
    ];

    #[test]
    fn a_file_opened_from_the_tree_is_the_preview_until_another_or_a_kept_open_takes_it() {
        for (name, steps, expected, tabs) in CASES {
            let shown = opened(steps);
            assert_eq!(shown, (expected.map(str::to_owned), *tabs), "{name}");
        }
    }

    fn opened(steps: &[Step]) -> (Option<String>, usize) {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        for step in steps {
            match step {
                Step::Preview(file) => rig.shell.preview_spec(spec(file)),
                Step::Open(file) => rig.shell.open_spec(spec(file)),
            }
        }
        let tabs = rig.shell.showing().unwrap().pane_ids().len();
        (preview_of(&rig.shell), tabs)
    }

    fn spec(file: &str) -> PaneSpec {
        PaneSpec::editor("t1", file).unwrap()
    }

    #[test]
    fn a_first_edit_keeps_the_preview_as_an_ordinary_tab() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        rig.shell
            .preview_spec(PaneSpec::editor("t1", "a.rs").unwrap());
        resync(&mut rig);
        let tab = rig.shell.showing().unwrap();
        let pane = tab.pane_ids().into_iter().last().unwrap();
        rig.shell.mark_dirty(pane);
        assert_eq!(preview_of(&rig.shell), None);
    }
}
