//! What the projects scope of one host currently says.

use std::collections::HashSet;

use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::chat_record::ChatRecord;
use crate::folder_record::FolderRecord;
use crate::frame::Frame;
use crate::operation_record::OperationRecord;
use crate::project_record::ProjectRecord;
use crate::repo_record::RepoRecord;
use crate::table::Table;
use crate::workspace_record::WorkspaceRecord;

pub(crate) const PROJECTS_SCOPE: &str = "projects";

#[derive(Debug, Clone, Default)]
pub(crate) struct EntityStore {
    pub(crate) projects: Table<ProjectRecord>,
    pub(crate) repos: Table<RepoRecord>,
    pub(crate) workspaces: Table<WorkspaceRecord>,
    pub(crate) chats: Table<ChatRecord>,
    pub(crate) folders: Table<FolderRecord>,
    pub(crate) operations: Table<OperationRecord>,
    epoch: Option<String>,
    unread: Vec<String>,
    fresh: Option<HashSet<String>>,
    synced: bool,
}

impl EntityStore {
    pub(crate) fn synced(&self) -> bool {
        self.synced
    }

    pub(crate) fn unread(&self) -> &[String] {
        &self.unread
    }

    pub(crate) fn begin_snapshot(&mut self) {
        self.fresh = Some(HashSet::new());
    }

    pub(crate) fn apply(
        &mut self,
        frame: Frame,
    ) {
        match frame {
            Frame::Ready { epoch } => self.new_epoch(epoch),
            Frame::Entity {
                scope,
                kind,
                id,
                v,
                data,
                gone,
            } if scope == PROJECTS_SCOPE => self.entity((&kind, &id), v, (data, gone)),
            Frame::Synced { scope, unread } if scope == PROJECTS_SCOPE => self.finish(&unread),
            _ => {},
        }
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
        (kind, id): (&str, &str),
        version: u64,
        (data, gone): (Option<Value>, bool),
    ) {
        if let Some(fresh) = self.fresh.as_mut() {
            fresh.insert(format!("{kind}:{id}"));
        }
        if gone {
            self.forget(kind, id);
            return;
        }
        let Some(data) = data else {
            return;
        };
        match kind {
            "project" => put(&mut self.projects, (id, version), data),
            "repo" => put(&mut self.repos, (id, version), data),
            "workspace" => put(&mut self.workspaces, (id, version), data),
            "chat" => put(&mut self.chats, (id, version), data),
            "folder" => put(&mut self.folders, (id, version), data),
            "operation" => put(&mut self.operations, (id, version), data),
            _ => {},
        }
    }

    fn forget(
        &mut self,
        kind: &str,
        id: &str,
    ) {
        match kind {
            "project" => self.projects.remove(id),
            "repo" => self.repos.remove(id),
            "workspace" => self.workspaces.remove(id),
            "chat" => self.chats.remove(id),
            "folder" => self.folders.remove(id),
            "operation" => self.operations.remove(id),
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
        self.projects.retain(|id, _| held("project", id));
        self.repos.retain(|id, _| held("repo", id));
        self.operations.retain(|id, _| held("operation", id));
        self.workspaces.retain(|id, row| {
            held("workspace", id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
        });
        self.chats.retain(|id, row| {
            held("chat", id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
        });
        self.folders.retain(|id, row| {
            held("folder", id) || unreadable(&row.repo_id) || unreadable(&row.project_id)
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
    use crate::frame::decode;

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

    fn seen(store: &EntityStore) -> String {
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

    fn landing(store: &mut EntityStore) {
        store.begin_snapshot();
        store.apply(entity("project", "p", 1, json!({"id": "p", "name": "A"})));
    }

    fn landed(store: &mut EntityStore) {
        landing(store);
        store.apply(synced(&[]));
    }

    fn resnapshot(store: &mut EntityStore) {
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

    fn older(store: &mut EntityStore) {
        store.apply(entity("chat", "c", 5, json!({"id": "c", "title": "new"})));
        store.apply(entity("chat", "c", 4, json!({"id": "c", "title": "old"})));
    }

    fn removed(store: &mut EntityStore) {
        older(store);
        let gone =
            decode(r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":6,"gone":true}"#);
        store.apply(gone.unwrap());
    }

    fn new_epoch(store: &mut EntityStore) {
        store.apply(Frame::Ready { epoch: "a".into() });
        store.apply(entity("chat", "c", 9, json!({"id": "c", "title": "x"})));
        store.apply(Frame::Ready { epoch: "b".into() });
        store.apply(entity("chat", "c", 1, json!({"id": "c", "title": "y"})));
    }

    fn unreadable(store: &mut EntityStore) {
        store.apply(entity("chat", "c", 1, json!({"id": 7})));
    }

    fn other_scope(store: &mut EntityStore) {
        store.apply(Frame::Synced {
            scope: "ws:w".into(),
            unread: Vec::new(),
        });
    }

    type Case = (&'static str, fn(&mut EntityStore), &'static str);

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
            let mut store = EntityStore::default();
            play(&mut store);
            assert_eq!(seen(&store), expected, "{name}");
        }
    }
}
