//! What the projects scope of one host currently says.

mod chat_record;
mod departures;
mod folder_record;
mod operation_record;
mod pending_move;
mod placement_answer;
mod presence_record;
mod project_record;
mod repo_record;
mod stamp;
mod table;
mod workspace_record;

pub(crate) use chat_record::ChatRecord;
pub(crate) use folder_record::FolderRecord;
pub(crate) use operation_record::{Item, OperationRecord};
pub(crate) use placement_answer::PlacementAnswer;
pub(crate) use project_record::ProjectRecord;
pub(crate) use repo_record::RepoRecord;
pub(crate) use workspace_record::WorkspaceRecord;

use std::collections::HashSet;

use daemonapi::Frame;
use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::live::held::entities::presence_record::PresenceRecord;
use crate::live::held::entities::stamp::Stamp;
use crate::live::held::entities::table::Table;

pub(crate) const PROJECTS_SCOPE: &str = "projects";
const PROJECT: &str = "project";
const REPO: &str = "repo";
const WORKSPACE: &str = "workspace";
const CHAT: &str = "chat";
const FOLDER: &str = "folder";
const OPERATION: &str = "operation";
const PRESENCE: &str = "presence";

#[derive(Debug, Clone, Default)]
pub(crate) struct Entities {
    pub(crate) projects: Table<ProjectRecord>,
    pub(crate) repos: Table<RepoRecord>,
    pub(crate) workspaces: Table<WorkspaceRecord>,
    pub(crate) chats: Table<ChatRecord>,
    pub(crate) folders: Table<FolderRecord>,
    pub(crate) operations: Table<OperationRecord>,
    pub(crate) presence: Table<PresenceRecord>,
    epoch: Option<String>,
    unread: Vec<String>,
    fresh: Option<HashSet<String>>,
    synced: bool,
    departed: Vec<String>,
}

impl Entities {
    pub(crate) fn synced(&self) -> bool {
        self.synced
    }

    pub(crate) fn unread(&self) -> &[String] {
        &self.unread
    }

    pub(crate) fn begin_snapshot(&mut self) {
        self.fresh = Some(HashSet::new());
    }

    pub(crate) fn abandon_snapshot(&mut self) -> bool {
        self.fresh.take().is_some()
    }

    pub(crate) fn apply(
        &mut self,
        frame: Frame,
    ) {
        match frame {
            Frame::Ready { epoch } => self.new_epoch(epoch),
            Frame::Entity { ref scope, .. } if scope == PROJECTS_SCOPE => self.entity_frame(frame),
            Frame::Synced { scope, unread } if scope == PROJECTS_SCOPE => self.finish(&unread),
            _ => {},
        }
    }

    fn entity_frame(
        &mut self,
        frame: Frame,
    ) {
        let Frame::Entity {
            kind,
            id,
            v,
            data,
            gone,
            ..
        } = frame
        else {
            return;
        };
        let stamp = Stamp {
            kind: &kind,
            id: &id,
            version: v,
            gone,
        };
        self.entity(&stamp, data);
    }

    fn new_epoch(
        &mut self,
        epoch: String,
    ) {
        if self.epoch.as_deref() == Some(epoch.as_str()) {
            return;
        }
        self.epoch = Some(epoch);
        self.projects.void_versions();
        self.repos.void_versions();
        self.workspaces.void_versions();
        self.chats.void_versions();
        self.folders.void_versions();
        self.operations.void_versions();
    }

    fn entity(
        &mut self,
        stamp: &Stamp<'_>,
        data: Option<Value>,
    ) {
        let &Stamp {
            kind,
            id,
            version,
            gone,
        } = stamp;
        if let Some(fresh) = self.fresh.as_mut() {
            fresh.insert(format!("{kind}:{id}"));
        }
        if gone {
            return self.forget(kind, id);
        }
        let Some(data) = data else {
            return;
        };
        match kind {
            PROJECT => put(&mut self.projects, (id, version), data),
            REPO => put(&mut self.repos, (id, version), data),
            WORKSPACE => put(&mut self.workspaces, (id, version), data),
            CHAT => put(&mut self.chats, (id, version), data),
            FOLDER => put(&mut self.folders, (id, version), data),
            OPERATION => put(&mut self.operations, (id, version), data),
            PRESENCE => put(&mut self.presence, (id, version), data),
            _ => {},
        }
    }

    fn forget(
        &mut self,
        kind: &str,
        id: &str,
    ) {
        match kind {
            PROJECT => self.projects.remove(id),
            REPO => self.repos.remove(id),
            WORKSPACE => self.workspaces.remove(id),
            CHAT => self.forget_chat(id),
            FOLDER => self.folders.remove(id),
            OPERATION => self.operations.remove(id),
            PRESENCE => self.presence.remove(id),
            _ => {},
        }
    }

    fn finish(
        &mut self,
        unread: &[String],
    ) {
        if let Some(fresh) = self.fresh.take() {
            self.prune(&fresh, unread);
        }
        self.unread = unread.to_vec();
        self.synced = true;
    }

    fn prune(
        &mut self,
        fresh: &HashSet<String>,
        unread: &[String],
    ) {
        let held = |kind: &str, id: &str| fresh.contains(&format!("{kind}:{id}"));
        let unreadable = |owner: &str| unread.iter().any(|name| name == owner);
        self.projects.retain(|id, _| held(PROJECT, id));
        self.repos.retain(|id, _| held(REPO, id));
        self.operations.retain(|id, _| held(OPERATION, id));
        self.presence.retain(|id, _| held(PRESENCE, id));
        self.workspaces.retain(|id, row| {
            held(WORKSPACE, id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
        });
        self.prune_chats(|id, row| {
            held(CHAT, id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
        });
        self.folders.retain(|id, row| {
            held(FOLDER, id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
        });
    }
}

fn put<T: DeserializeOwned>(
    table: &mut Table<T>,
    (id, version): (&str, u64),
    data: Value,
) {
    match serde_json::from_value(data) {
        Ok(row) => table.put(id, version, row),
        Err(error) => tracing::warn!(%error, id, "an entity frame did not parse and was skipped"),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn entity(
        kind: &str,
        id: &str,
        v: u64,
        data: Value,
    ) -> Frame {
        Frame::Entity {
            scope: PROJECTS_SCOPE.into(),
            kind: kind.into(),
            id: id.into(),
            v,
            data: Some(data),
            gone: false,
        }
    }

    fn synced(unread: &[&str]) -> Frame {
        Frame::Synced {
            scope: PROJECTS_SCOPE.into(),
            unread: unread.iter().map(|name| (*name).to_owned()).collect(),
        }
    }

    fn seen(store: &Entities) -> String {
        let mut chats: Vec<String> = store
            .chats
            .values()
            .map(|chat| format!("{}={}", chat.id, chat.title))
            .collect();
        chats.sort_unstable();
        let projects: Vec<&str> = store.projects.values().map(|p| p.name.as_str()).collect();
        format!(
            "synced={} projects={projects:?} chats={chats:?}",
            store.synced()
        )
    }

    fn landing(store: &mut Entities) {
        store.begin_snapshot();
        store.apply(entity("project", "p", 1, json!({"id": "p", "name": "A"})));
    }

    fn landed(store: &mut Entities) {
        landing(store);
        store.apply(synced(&[]));
    }

    fn resnapshot(store: &mut Entities) {
        store.apply(entity(
            "chat",
            "kept",
            1,
            json!({"id": "kept", "repoId": "r"}),
        ));
        store.apply(entity(
            "chat",
            "gone",
            1,
            json!({"id": "gone", "repoId": "s"}),
        ));
        store.begin_snapshot();
        store.apply(synced(&["r"]));
    }

    fn older(store: &mut Entities) {
        store.apply(entity("chat", "c", 5, json!({"id": "c", "title": "new"})));
        store.apply(entity("chat", "c", 4, json!({"id": "c", "title": "old"})));
    }

    fn removed(store: &mut Entities) {
        older(store);
        let gone = Frame::decode(
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":6,"gone":true}"#,
        );
        store.apply(gone.unwrap());
    }

    fn new_epoch(store: &mut Entities) {
        store.apply(Frame::Ready { epoch: "a".into() });
        store.apply(entity("chat", "c", 9, json!({"id": "c", "title": "x"})));
        store.apply(Frame::Ready { epoch: "b".into() });
        store.apply(entity("chat", "c", 1, json!({"id": "c", "title": "y"})));
    }

    fn unreadable(store: &mut Entities) {
        store.apply(entity("chat", "c", 1, json!({"id": 7})));
    }

    fn other_scope(store: &mut Entities) {
        store.apply(Frame::Synced {
            scope: "ws:w".into(),
            unread: Vec::new(),
        });
    }

    type Case = (&'static str, fn(&mut Entities), &'static str);

    const CASES: [Case; 8] = [
        (
            "rows arrive before the sync",
            landing,
            r#"synced=false projects=["A"] chats=[]"#,
        ),
        (
            "a snapshot lands and syncs",
            landed,
            r#"synced=true projects=["A"] chats=[]"#,
        ),
        (
            "a new snapshot drops what it leaves out but keeps unread owners",
            resnapshot,
            r#"synced=true projects=[] chats=["kept="]"#,
        ),
        (
            "an older version is ignored",
            older,
            r#"synced=false projects=[] chats=["c=new"]"#,
        ),
        (
            "a removal forgets the row",
            removed,
            "synced=false projects=[] chats=[]",
        ),
        (
            "a new epoch accepts lower versions",
            new_epoch,
            r#"synced=false projects=[] chats=["c=y"]"#,
        ),
        (
            "an unreadable payload is skipped",
            unreadable,
            "synced=false projects=[] chats=[]",
        ),
        (
            "another scope's sync is ignored",
            other_scope,
            "synced=false projects=[] chats=[]",
        ),
    ];

    #[test]
    fn frames_build_the_store_as_the_daemon_meant() {
        for (name, play, expected) in CASES {
            let mut store = Entities::default();
            play(&mut store);
            assert_eq!(seen(&store), expected, "{name}");
        }
    }
}
