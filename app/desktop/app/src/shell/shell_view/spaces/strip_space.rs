use gpui::Context;
use layout::{Command, Layout, SpaceId};
use sidebar::Sink as _;

use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn apply_from_strip(
        &mut self,
        commands: &[Command],
        cx: &mut Context<'_, Self>,
    ) {
        let active = self.state.space();
        let commands = made_in(commands, &active);
        let visited = space_to_visit(&commands, self.state.layout(), &active);
        self.apply_guarded(commands, cx);
        let Some(space) = visited else {
            return;
        };
        let select = sidebar::Command::SelectProject(space.as_str().into());
        if let Err(error) = self.data.commands().dispatch(select) {
            tracing::warn!(%error, "the switch to the tab's space was refused");
        }
    }
}

fn made_in(
    commands: &[Command],
    active: &str,
) -> Vec<Command> {
    commands
        .iter()
        .cloned()
        .map(|command| in_active(command, active))
        .collect()
}

fn in_active(
    command: Command,
    active: &str,
) -> Command {
    match command {
        Command::AddTab { id, title, space } if space.is_unowned() => Command::AddTab {
            id,
            title,
            space: SpaceId::new(active),
        },
        other => other,
    }
}

fn space_to_visit(
    commands: &[Command],
    layout: &Layout,
    active: &str,
) -> Option<String> {
    let activated = commands.iter().find_map(|command| match command {
        Command::ActivateTab { tab } => layout.tab(*tab),
        _ => None,
    })?;
    let space = activated.space();
    (!space.is_empty() && space != active).then(|| space.to_owned())
}

#[cfg(test)]
mod tests {
    use layout::TabId;

    use super::*;

    fn add(
        id: u64,
        space: &str,
    ) -> Command {
        Command::AddTab {
            id: TabId(id),
            title: String::new(),
            space: SpaceId::new(space),
        }
    }

    fn layout() -> Layout {
        let mut layout = Layout::default();
        for (id, space) in [(1, "p1"), (2, "p2"), (3, "")] {
            layout.apply(add(id, space)).unwrap();
        }
        layout
    }

    type Case = (&'static str, u64, Option<&'static str>);

    const CASES: &[Case] = &[
        ("a tab of another space", 2, Some("p2")),
        ("a tab of the active space", 1, None),
        ("a tab with no space yet", 3, None),
    ];

    #[test]
    fn activating_a_tab_of_another_space_visits_that_space() {
        for (name, tab, visited) in CASES {
            let activate = [Command::ActivateTab { tab: TabId(*tab) }];
            let space = space_to_visit(&activate, &layout(), "p1");
            assert_eq!(space.as_deref(), *visited, "{name}");
        }
    }

    #[test]
    fn a_tab_the_strip_makes_belongs_to_the_active_space() {
        let made = made_in(&[add(9, "")], "p1");
        let space = match made.first() {
            Some(Command::AddTab { space, .. }) => space.as_str(),
            _ => "",
        };
        assert_eq!(space, "p1", "stamped");
    }
}
