//! A branch filed somewhere else: its fork parent moved when the new container's anchor differs,
//! then its row placed.
//!
//! Spec: develop `web/src/components/sidebar/lib/drop-plan-tree.ts` (`planTreeRowDrop`).

use daemonapi::{Call, Route};
use serde_json::{Map, Value, json};

use crate::live::{Entities, PlanError, WorkspaceRecord};
use crate::plan::Plan;
use crate::plan::after::After;
use crate::plan::relocate::container::Container;
use crate::plan::relocate::slot::Slot;

const PLACEHOLDER: &str = "placeholder";
const PARENT: &str = "parentId";
const ORDER: &str = "order";

pub(crate) struct BranchDrop<'a> {
    pub(crate) store: &'a Entities,
    pub(crate) host: &'a str,
    pub(crate) ws: &'a WorkspaceRecord,
}

impl BranchDrop<'_> {
    pub(crate) fn plan(
        &self,
        slot: &Slot,
    ) -> Result<Plan, PlanError> {
        let container = self.container_of(&slot.parent);
        let current = self.ws.parent_id.as_str();
        let next = match &container {
            Container::Root => String::new(),
            Container::Workspace(id) | Container::Folder(id) => self.anchor(id),
        };
        let direct = (Some(self.direct_parent(&container)), slot.index);
        let shown = Some(current)
            .filter(|id| self.listed(id))
            .unwrap_or_default();
        if next == shown {
            return Ok(self.placed(direct));
        }
        let fork = Some(next)
            .filter(|id| !id.is_empty())
            .unwrap_or_else(|| self.home());
        if fork.is_empty() || fork == current {
            return Ok(self.placed(direct));
        }
        self.reparented(&fork, (&container, slot.index))
    }

    fn reparented(
        &self,
        fork: &str,
        (container, index): (&Container, usize),
    ) -> Result<Plan, PlanError> {
        let placeholder = self.store.workspaces.get(fork);
        if placeholder.is_some_and(|parent| parent.provisioning == PLACEHOLDER) {
            return Err(PlanError::ForkNotCheckedOut);
        }
        let owner = self.ws.owning_chat_id.as_str();
        if owner.is_empty() {
            return Err(PlanError::ChatLoading);
        }
        let operation = daemonapi::Uuid::random().to_string();
        let chats = Route::chats(&self.ws.project_id, &self.ws.repo_id);
        let reparent = Call::post(format!("{chats}/{owner}/reparent"))
            .json(json!({"newParentId": fork}))
            .idempotent(&operation);
        let folder = match container {
            Container::Folder(id) => Some(id.clone()),
            Container::Root | Container::Workspace(_) => None,
        };
        let mut plan = self.placed((folder, index));
        plan.calls.insert(0, reparent);
        Ok(plan.settling(operation))
    }

    fn placed(
        &self,
        (parent, index): (Option<String>, usize),
    ) -> Plan {
        let mut body = Map::new();
        if let Some(parent) = parent {
            body.insert(PARENT.into(), Value::String(parent));
        }
        body.insert(ORDER.into(), json!(index));
        let scope = Route::scope(&self.ws.project_id, &self.ws.repo_id);
        let path = format!("{scope}/workspaces/{}/placement", self.ws.id);
        Plan::of(self.host, vec![Call::patch(path).json(Value::Object(body))])
            .then(After::Place(self.ws.id.clone()))
    }

    fn direct_parent(
        &self,
        container: &Container,
    ) -> String {
        match container {
            Container::Root => String::new(),
            Container::Folder(id) => id.clone(),
            Container::Workspace(id) => self.owner_row(id),
        }
    }

    fn owner_row(
        &self,
        workspace: &str,
    ) -> String {
        self.store
            .workspaces
            .get(workspace)
            .map(|ws| ws.owning_chat_id.clone())
            .filter(|owner| !owner.is_empty())
            .unwrap_or_else(|| workspace.to_owned())
    }

    fn container_of(
        &self,
        row: &str,
    ) -> Container {
        let id = self.workspace_of_row(row).unwrap_or_else(|| row.to_owned());
        if self.listed(&id) {
            return Container::Workspace(id);
        }
        let filed = self.store.folders.get(&id);
        if filed.is_some_and(|folder| folder.repo_id == self.ws.repo_id) {
            return Container::Folder(id);
        }
        Container::Root
    }

    fn anchor(
        &self,
        container: &str,
    ) -> String {
        let up = |id: &String| self.store.folders.get(id).map(|f| f.parent_id.clone());
        let hops = self.store.folders.values().count() + 1;
        std::iter::successors(Some(container.to_owned()), up)
            .take_while(|id| !id.is_empty())
            .take(hops)
            .find_map(|id| self.workspace_at(id))
            .unwrap_or_default()
    }

    fn workspace_at(
        &self,
        id: String,
    ) -> Option<String> {
        if self.listed(&id) {
            return Some(id);
        }
        self.workspace_of_row(&id)
    }

    fn workspace_of_row(
        &self,
        row: &str,
    ) -> Option<String> {
        self.store
            .workspaces
            .values()
            .find(|other| other.owning_chat_id == row && self.sibling(other))
            .map(|other| other.id.clone())
    }

    fn listed(
        &self,
        id: &str,
    ) -> bool {
        self.store
            .workspaces
            .get(id)
            .is_some_and(|other| self.sibling(other))
    }

    fn sibling(
        &self,
        other: &WorkspaceRecord,
    ) -> bool {
        other.repo_id == self.ws.repo_id && !other.is_default
    }

    fn home(&self) -> String {
        self.store
            .workspaces
            .values()
            .find(|other| other.repo_id == self.ws.repo_id && other.is_default)
            .map_or_else(String::new, |other| other.id.clone())
    }
}

#[cfg(test)]
mod tests {
    use daemonapi::Frame;
    use sidebar::DropMode;

    use super::*;
    use crate::live::{Hub, OutcomeSink, State};
    use crate::plan::relocate::drop_row;
    use crate::plan::runner::run;
    use crate::test_support::{quiet, state};

    const BRANCH: &str = "/v0/projects/p/repos/r/workspaces/b/placement";
    const REPARENT: &str = "/v0/projects/p/repos/r/chats/bo/reparent";
    const SHIFTED_ORDER: i64 = 3;

    fn entity(
        (kind, id): (&str, &str),
        data: Value,
    ) -> Frame {
        Frame::Entity {
            scope: "projects".into(),
            kind: kind.into(),
            id: id.into(),
            v: 1,
            data: Some(data),
            gone: false,
        }
    }

    fn with_a_second_branch(provisioning: &str) -> State {
        let (mut state, _) = state();
        let ws = json!({"id": "c", "projectId": "p", "repoId": "r", "branch": "other",
            "owningChatId": "co", "provisioning": provisioning});
        let owner = json!({"id": "co", "projectId": "p", "repoId": "r", "workspaceId": "c",
            "title": "Other", "createdAt": "2026-01-05T00:00:00Z"});
        state.apply("local", entity(("workspace", "c"), ws));
        state.apply("local", entity(("chat", "co"), owner));
        state
    }

    fn sent(plan: &Plan) -> Vec<(&str, &str, Value)> {
        let body = |call: &Call| call.body.clone().unwrap_or_default();
        plan.calls
            .iter()
            .map(|call| (call.method, call.path.as_str(), body(call)))
            .collect()
    }

    fn filed_at_its_own_fork_anchor_is_only_placed() {
        let (state, _) = state();
        let plan = drop_row(&state, ("bo", "f"), DropMode::Into).unwrap();
        let body = json!({"parentId": "f", "order": 0});
        assert_eq!(sent(&plan), [("PATCH", BRANCH, body)]);
        assert_eq!(plan.after, After::Place("b".into()));
        assert_eq!(plan.settling, None);
    }

    fn nested_under_another_branch_is_reparented_then_placed() {
        let state = with_a_second_branch("");
        let plan = drop_row(&state, ("bo", "co"), DropMode::Into).unwrap();
        let want = [
            ("POST", REPARENT, json!({"newParentId": "c"})),
            ("PATCH", BRANCH, json!({"order": 0})),
        ];
        assert_eq!(sent(&plan), want);
        let headers = &plan.calls[0].headers;
        let key = headers.iter().find(|(name, _)| name == "idempotency-key");
        assert_eq!(key.map(|(_, id)| id), plan.settling.as_ref());
    }

    fn a_fork_parent_without_a_checkout_is_refused_before_any_call() {
        let state = with_a_second_branch(PLACEHOLDER);
        let refused = drop_row(&state, ("bo", "co"), DropMode::Into).unwrap_err();
        assert!(matches!(refused, PlanError::ForkNotCheckedOut));
    }

    const CASES: [fn(); 3] = [
        filed_at_its_own_fork_anchor_is_only_placed,
        nested_under_another_branch_is_reparented_then_placed,
        a_fork_parent_without_a_checkout_is_refused_before_any_call,
    ];

    #[test]
    fn a_dropped_branch_moves_its_fork_parent_only_when_the_anchor_changes() {
        for case in CASES {
            case();
        }
    }

    type Filed = (Option<(String, i64)>, Option<i64>);

    fn filed(state: &State) -> Filed {
        let store = &state.hosts[0].store;
        let ws = store.workspaces.get("b");
        let ws = ws.map(|ws| (ws.folder_id.clone(), ws.order));
        (ws, store.chats.get("hc").map(|chat| chat.order))
    }

    #[tokio::test]
    async fn the_placement_answer_moves_the_branch_and_its_shifted_siblings() {
        let (state, daemon) = state();
        let moved = json!({"id": "bo", "parentId": "f", "order": 0});
        let shifted = json!([{"id": "hc", "parentId": "", "order": SHIFTED_ORDER}]);
        let answer = json!({"workspace": moved, "shifted": shifted}).to_string();
        daemon.ok("PATCH", BRANCH, &answer);
        let plan = drop_row(&state, ("bo", "f"), DropMode::Into).unwrap();
        let hub = Hub::new(state, quiet());
        let outcome: OutcomeSink = std::sync::Arc::new(drop::<crate::live::Outcome>);
        run(&hub, plan, &outcome).await;
        let want = (Some(("f".to_owned(), 0)), Some(SHIFTED_ORDER));
        assert_eq!(hub.read(filed), want);
    }
}
