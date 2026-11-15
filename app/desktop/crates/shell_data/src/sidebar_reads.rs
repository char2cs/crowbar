//! What the sidebar reads, derived from the shared state.

use sidebar::{
    Changes, FileRow, Focus, Host, HostId, HostKind, MoveTarget, Notice, Project, ProjectId,
    ProjectTree, Reach, RowKind, ScopeSync,
};

use crate::file_rows;
use crate::host_slot::HostSlot;
use crate::link_state::LinkState;
use crate::pending_rows;
use crate::project_rows;
use crate::repo_mark;
use crate::state::State;

pub(crate) fn shown(state: &State) -> Option<&HostSlot> {
    let selected = state.selected_host.as_deref();
    selected
        .and_then(|id| state.hosts.iter().find(|slot| slot.spec.id == id))
        .or_else(|| state.hosts.first())
}

pub(crate) fn host_of<'a>(
    state: &'a State,
    project: &str,
) -> Option<&'a HostSlot> {
    state
        .hosts
        .iter()
        .find(|slot| slot.store.projects.get(project).is_some())
}

pub(crate) fn sync(state: &State) -> ScopeSync {
    if shown(state).is_some_and(|slot| slot.store.synced()) {
        return ScopeSync::Ready;
    }
    ScopeSync::Pending
}

pub(crate) fn hosts(state: &State) -> Vec<Host> {
    state.hosts.iter().map(host_of_slot).collect()
}

fn host_of_slot(slot: &HostSlot) -> Host {
    Host {
        id: HostId::from(slot.spec.id.as_str()),
        name: slot.spec.name.clone(),
        kind: host_kind(slot),
        state: slot.link.host_state(),
        endpoint: slot.spec.endpoint.clone(),
    }
}

fn host_kind(slot: &HostSlot) -> HostKind {
    if slot.spec.endpoint.is_some() {
        return HostKind::Remote;
    }
    HostKind::Local
}

fn reach_of(slot: &HostSlot) -> Reach {
    if slot.link == LinkState::Down {
        return Reach::Unreachable;
    }
    Reach::Reachable
}

pub(crate) fn projects(state: &State) -> Vec<Project> {
    let Some(slot) = shown(state) else {
        return Vec::new();
    };
    let mut records: Vec<_> = slot.store.projects.values().collect();
    records.sort_by(|a, b| (a.order, &a.id).cmp(&(b.order, &b.id)));
    records
        .into_iter()
        .map(|record| Project {
            id: ProjectId::from(record.id.as_str()),
            name: record.name.clone(),
            mark: repo_mark::of((&record.name, ""), (&record.avatar_emoji, "")),
            host: HostId::from(slot.spec.id.as_str()),
        })
        .collect()
}

pub(crate) fn active_project(state: &State) -> Option<ProjectId> {
    let listed = projects(state);
    let wanted = state.active_project.as_deref();
    let chosen = listed
        .iter()
        .find(|project| Some(project.id.as_str()) == wanted);
    chosen
        .or_else(|| listed.first())
        .map(|project| project.id.clone())
}

pub(crate) fn tree(
    state: &State,
    project: &ProjectId,
) -> ProjectTree {
    let Some(slot) = host_of(state, project.as_str()) else {
        return ProjectTree::default();
    };
    let mut pending = pending_rows::of(&slot.store);
    pending.extend(state.local_rows.iter().cloned());
    ProjectTree {
        rows: project_rows::of(&slot.store, project.as_str(), &pending),
        notice: notice(slot, project.as_str()),
        reach: reach_of(slot),
    }
}

fn notice(
    slot: &HostSlot,
    project: &str,
) -> Option<Notice> {
    let unread = slot.store.unread();
    if unread.iter().any(|owner| owner == project) {
        return Some(Notice::Project);
    }
    slot.store
        .repos
        .values()
        .find(|repo| repo.project_id == project && unread.contains(&repo.id))
        .map(|repo| Notice::Repo(repo.name.clone()))
}

pub(crate) fn focus(state: &State) -> Focus {
    let Some(chat) = state.panels.chat.as_deref() else {
        return Focus::Nothing;
    };
    let record = state
        .hosts
        .iter()
        .find_map(|slot| slot.store.chats.get(chat));
    match record {
        None => Focus::Nothing,
        Some(chat) if chat.repo_id.is_empty() => Focus::Workspace,
        Some(_) => Focus::Repository,
    }
}

pub(crate) fn move_targets(
    state: &State,
    row: &sidebar::RowId,
) -> Vec<MoveTarget> {
    let Some(project) = active_project(state) else {
        return Vec::new();
    };
    let rows = tree(state, &project).rows;
    let Some(at) = rows.iter().position(|candidate| &candidate.id == row) else {
        return Vec::new();
    };
    let depth = rows.get(at).map_or(0, |moved| moved.depth);
    let inside = rows
        .iter()
        .skip(at + 1)
        .take_while(|below| below.depth > depth)
        .count();
    rows.iter()
        .enumerate()
        .filter(|(index, _)| *index < at || *index > at + inside)
        .filter(|(_, candidate)| matches!(candidate.kind, RowKind::Folder | RowKind::Branch))
        .map(|(_, candidate)| MoveTarget {
            id: candidate.id.clone(),
            label: candidate.label.clone(),
            kind: candidate.kind,
        })
        .collect()
}

pub(crate) fn files(state: &State) -> Vec<FileRow> {
    file_rows::of(&state.panels)
}

pub(crate) fn changes(state: &State) -> Changes {
    let mut changes = state.panels.changes.clone();
    changes.parent = crate::files_reads::followed_workspace(state)
        .and_then(|workspace| crate::merge_reads::parent_branch(state, workspace));
    changes
}

#[cfg(test)]
mod tests {
    use sidebar::{Focus, HostState, ProjectId, Reach, RowId, ScopeSync};

    use super::*;
    use crate::entity_store::EntityStore;
    use crate::link_state::LinkState;
    use crate::test_support::state;

    type Check = fn(&mut State);

    fn synced_host_lists_spaces(state: &mut State) {
        assert_eq!(sync(state), ScopeSync::Ready);
        assert_eq!(projects(state).len(), 1);
        assert_eq!(active_project(state), Some(ProjectId::from("p")));
        let hosts = hosts(state);
        assert_eq!(
            (hosts[0].name.as_str(), hosts[0].state),
            ("This computer", HostState::Probing)
        );
    }

    fn tree_reach_follows_the_stream(state: &mut State) {
        assert_eq!(tree(state, &ProjectId::from("p")).reach, Reach::Reachable);
        state.hosts[0].link = LinkState::Down;
        assert_eq!(tree(state, &ProjectId::from("p")).reach, Reach::Unreachable);
    }

    fn a_chat_decides_focus(state: &mut State) {
        assert_eq!(focus(state), Focus::Nothing);
        state.panels.follow(Some("t".into()));
        assert_eq!(focus(state), Focus::Repository);
        state.panels.follow(Some("hc".into()));
        assert_eq!(focus(state), Focus::Workspace);
    }

    fn move_targets_are_the_other_rows(state: &mut State) {
        let targets = move_targets(state, &RowId::from("t"));
        let ids: Vec<&str> = targets.iter().map(|target| target.id.as_str()).collect();
        assert!(ids.contains(&"f") && ids.contains(&"bo"));
        assert!(!ids.contains(&"t"));
    }

    fn an_unsynced_store_is_pending(state: &mut State) {
        state.hosts[0].store = EntityStore::default();
        assert_eq!(sync(state), ScopeSync::Pending);
        assert_eq!(projects(state).len(), 0);
    }

    const CASES: [Check; 5] = [
        synced_host_lists_spaces,
        tree_reach_follows_the_stream,
        a_chat_decides_focus,
        move_targets_are_the_other_rows,
        an_unsynced_store_is_pending,
    ];

    #[test]
    fn every_read_answers_from_the_state() {
        for check in CASES {
            let (mut state, _) = state();
            check(&mut state);
        }
    }
}
