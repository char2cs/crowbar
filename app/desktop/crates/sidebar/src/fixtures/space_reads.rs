//! What the demo data answers about its spaces, hosts and rows.

use crate::fixtures::demo_shell::DemoShell;
use crate::shell::ActiveSpace;
use crate::shell::AvatarTint;
use crate::shell::Command;
use crate::shell::Focus;
use crate::shell::Host;
use crate::shell::HostChoice;
use crate::shell::HostId;
use crate::shell::HostKind;
use crate::shell::HostState;
use crate::shell::Mark;
use crate::shell::MoveTarget;
use crate::shell::PaneFocus;
use crate::shell::Project;
use crate::shell::ProjectId;
use crate::shell::ProjectTree;
use crate::shell::RowActions;
use crate::shell::RowId;
use crate::shell::RunningAgent;
use crate::shell::ScopeSync;
use crate::shell::SpaceList;

impl SpaceList for DemoShell {
    fn sync(&self) -> ScopeSync {
        self.sync
    }

    fn projects(&self) -> Vec<Project> {
        let names = [
            ("inv", Mark::Fallback),
            ("website", Mark::Fallback),
            (
                "api",
                Mark::Letter {
                    letter: 'A',
                    tint: AvatarTint::Emerald,
                },
            ),
        ];
        names
            .into_iter()
            .take(self.spaces)
            .map(|(name, mark)| Project {
                id: ProjectId::from(name),
                name: name.to_owned(),
                mark,
                host: self.shown.clone(),
            })
            .collect()
    }
}

impl HostChoice for DemoShell {
    fn hosts(&self) -> Vec<Host> {
        let local = Host {
            id: HostId::from("local"),
            name: "This computer".into(),
            kind: HostKind::Local,
            state: HostState::Up,
            endpoint: None,
        };
        if self.hosts < 2 {
            return vec![local];
        }
        let remote = Host {
            id: HostId::from("devbox"),
            name: "devbox".into(),
            kind: HostKind::Remote,
            state: HostState::Down,
            endpoint: Some("devbox.internal:7421".into()),
        };
        vec![local, remote]
    }

    fn shown_host(&self) -> Option<HostId> {
        Some(self.shown.clone())
    }
}

impl ActiveSpace for DemoShell {
    fn active_project(&self) -> Option<ProjectId> {
        Some(self.active.borrow().clone())
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        let mut tree = ProjectTree::default();
        if project == &*self.active.borrow() {
            tree.rows.clone_from(&self.rows.borrow());
        }
        tree
    }
}

impl PaneFocus for DemoShell {
    fn focus(&self) -> Focus {
        self.focus
    }

    fn running_agents(&self) -> Vec<RunningAgent> {
        self.running.clone()
    }

    fn can_start_terminal_thread(&self) -> bool {
        true
    }
}

impl RowActions for DemoShell {
    fn move_targets(
        &self,
        _row: &RowId,
    ) -> Vec<MoveTarget> {
        Vec::new()
    }

    fn serves(
        &self,
        command: &Command,
    ) -> bool {
        !self.refused.contains(command)
    }

    fn can_remove_folder(
        &self,
        path: &str,
    ) -> bool {
        self.removable.iter().any(|folder| folder == path)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type CountCase = (&'static str, fn() -> DemoShell, usize, usize);

    const COUNT_CASES: &[CountCase] = &[
        ("the reference data", DemoShell::reference, 2, 1),
        ("the rich data", DemoShell::rich, 3, 2),
    ];

    #[test]
    fn the_data_lists_its_spaces_and_hosts() {
        for (name, build, spaces, hosts) in COUNT_CASES {
            let data = build();
            assert_eq!(data.projects().len(), *spaces, "{name}: spaces");
            assert_eq!(data.hosts().len(), *hosts, "{name}: hosts");
        }
    }
}
