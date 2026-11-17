//! What the demo data answers about its spaces, hosts and rows.

use crate::active_space::ActiveSpace;
use crate::avatar_tint::AvatarTint;
use crate::command::Command;
use crate::demo_data::DemoData;
use crate::focus::Focus;
use crate::host::Host;
use crate::host_id::HostId;
use crate::host_kind::HostKind;
use crate::host_state::HostState;
use crate::mark::Mark;
use crate::move_target::MoveTarget;
use crate::pane_focus::PaneFocus;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row_actions::RowActions;
use crate::row_id::RowId;
use crate::running_agent::RunningAgent;
use crate::scope_sync::ScopeSync;
use crate::space_list::SpaceList;

impl SpaceList for DemoData {
    fn sync(&self) -> ScopeSync {
        self.sync
    }

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
                host: HostId::from("local"),
            })
            .collect()
    }
}

impl ActiveSpace for DemoData {
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

impl PaneFocus for DemoData {
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

impl RowActions for DemoData {
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

    type CountCase = (&'static str, fn() -> DemoData, usize, usize);

    const COUNT_CASES: &[CountCase] = &[
        ("the reference data", DemoData::reference, 2, 1),
        ("the rich data", DemoData::rich, 3, 2),
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
