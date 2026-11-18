//! Finding the entity a row id names.

mod target_kind;

pub(crate) use target_kind::TargetKind;

use crate::live::ChatRecord;
use crate::live::EntityStore;
use crate::live::PlanError;
use crate::live::State;
use crate::live::WorkspaceRecord;
use crate::rows::{scope_items, workspace_view};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Target {
    pub(crate) id: String,
    pub(crate) project: String,
    pub(crate) repo: String,
    pub(crate) kind: TargetKind,
}

impl Target {
    pub(crate) fn chat(&self) -> Result<&str, PlanError> {
        match &self.kind {
            TargetKind::Branch { owner, .. } if owner.is_empty() => Err(PlanError::ChatLoading),
            TargetKind::Branch { owner, .. } => Ok(owner),
            TargetKind::Folder => Err(PlanError::FolderHasNoChat),
            _ => Ok(&self.id),
        }
    }

    pub(crate) fn workspace(&self) -> Option<&str> {
        match &self.kind {
            TargetKind::RepoHeader { workspace }
            | TargetKind::Branch { workspace, .. }
            | TargetKind::Chat { workspace } => Some(workspace),
            TargetKind::Folder => None,
        }
    }
}

pub(crate) fn resolve(
    state: &State,
    id: &str,
) -> Option<Target> {
    state
        .hosts
        .iter()
        .find_map(|slot| resolve_in(&slot.store, id))
}

fn resolve_in(
    store: &EntityStore,
    id: &str,
) -> Option<Target> {
    if let Some(folder) = store.folders.get(id) {
        return Some(of(
            id,
            (&folder.project_id, &folder.repo_id),
            TargetKind::Folder,
        ));
    }
    if let Some(chat) = store.chats.get(id) {
        return Some(chat_target(store, chat));
    }
    let workspace = store.workspaces.get(id)?;
    Some(of(
        id,
        (&workspace.project_id, &workspace.repo_id),
        branch(workspace),
    ))
}

fn chat_target(
    store: &EntityStore,
    chat: &ChatRecord,
) -> Target {
    let scope = (chat.project_id.as_str(), chat.repo_id.as_str());
    let kind = chat_kind(store, chat);
    of(&chat.id, scope, kind)
}

fn of(
    id: &str,
    (project, repo): (&str, &str),
    kind: TargetKind,
) -> Target {
    Target {
        id: id.to_owned(),
        project: project.to_owned(),
        repo: repo.to_owned(),
        kind,
    }
}

fn branch(ws: &WorkspaceRecord) -> TargetKind {
    TargetKind::Branch {
        workspace: ws.id.clone(),
        owner: ws.owning_chat_id.clone(),
        locked: workspace_view::is_locked(ws),
    }
}

fn chat_kind(
    store: &EntityStore,
    chat: &ChatRecord,
) -> TargetKind {
    let checkout = scope_items::default_workspace(store, (&chat.project_id, &chat.repo_id));
    let header = checkout
        .filter(|ws| ws.owning_chat_id == chat.id && !chat.repo_id.is_empty())
        .map(|ws| TargetKind::RepoHeader {
            workspace: ws.id.clone(),
        });
    header.unwrap_or_else(|| owned_kind(store, chat))
}

fn owned_kind(
    store: &EntityStore,
    chat: &ChatRecord,
) -> TargetKind {
    let owned = store
        .workspaces
        .values()
        .find(|ws| ws.owning_chat_id == chat.id && !ws.is_default && !ws.repo_id.is_empty());
    owned.map_or_else(
        || TargetKind::Chat {
            workspace: chat.workspace_id.clone(),
        },
        branch,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_row_id_resolves_to_what_it_stands_for() {
        let (state, _) = state();
        let kind = |id: &str| resolve(&state, id).map(|target| target.kind);
        assert_eq!(kind("f"), Some(TargetKind::Folder));
        assert_eq!(
            kind("rdo"),
            Some(TargetKind::RepoHeader {
                workspace: "rd".into()
            })
        );
        assert_eq!(
            kind("bo"),
            Some(TargetKind::Branch {
                workspace: "b".into(),
                owner: "bo".into(),
                locked: false
            })
        );
        assert_eq!(
            kind("t"),
            Some(TargetKind::Chat {
                workspace: "b".into()
            })
        );
        assert_eq!(
            kind("hc"),
            Some(TargetKind::Chat {
                workspace: "hw".into()
            })
        );
        assert_eq!(kind("nope"), None);
    }

    #[test]
    fn a_branch_acts_through_its_owner_and_a_folder_has_no_chat() {
        let (state, _) = state();
        assert_eq!(resolve(&state, "bo").unwrap().chat().unwrap(), "bo");
        assert!(resolve(&state, "f").unwrap().chat().is_err());
        assert_eq!(resolve(&state, "t").unwrap().workspace(), Some("b"));
    }
}
