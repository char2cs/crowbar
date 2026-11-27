//! A chat filed somewhere else, moving to the workspace its new container runs in.
//!
//! Spec: develop `web/src/components/sidebar/lib/drop-plan.ts` (`planChatDrop`).

use daemonapi::{Call, Route};
use serde_json::{Map, Value, json};
use sidebar::{Row, RowKind, RowTree};

use crate::live::{PlanError, State};
use crate::plan::Plan;
use crate::plan::relocate::slot::Slot;
use crate::reads::host_of;
use crate::rows::scope_items;
use crate::rows::target::{Target, resolve};

pub(super) fn plan(
    state: &State,
    (rows, moved): (&[Row], &Target),
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let held = host_of(state, &moved.project).ok_or(PlanError::SpaceNotLoaded)?;
    let mut body = Map::new();
    body.insert("parentId".into(), json!(slot.parent));
    body.insert("order".into(), json!(slot.index));
    let container = rows.iter().position(|row| row.id.as_str() == slot.parent);
    let own = moved.workspace().unwrap_or_default();
    let dest = destination(state, (rows, moved), container);
    let known = !own.is_empty() && !dest.is_empty();
    let moves_workspace = known && own != dest;
    if moves_workspace {
        let name = held
            .store
            .projects
            .get(&moved.project)
            .map(|p| p.name.as_str());
        let row = container.and_then(|index| rows.get(index));
        body.insert("workspaceId".into(), Value::String(dest));
        let named = label(row, name.unwrap_or("the project home"));
        body.insert("label".into(), Value::String(named));
    }
    let chats = Route::chats(&moved.project, &moved.repo);
    let path = format!("{chats}/{}/placement", moved.chat()?);
    Ok(Plan::of(
        &held.spec.id,
        vec![Call::patch(path).json(Value::Object(body))],
    ))
}

fn destination(
    state: &State,
    (rows, moved): (&[Row], &Target),
    container: Option<usize>,
) -> String {
    let tree = RowTree::new(rows);
    let ground = tree.anchor(container).or_else(|| tree.repo(container));
    if let Some(row) = ground.and_then(|index| rows.get(index)) {
        let target = resolve(state, row.id.as_str());
        return target.map_or_default(|t| t.workspace().unwrap_or_default().to_owned());
    }
    let home = host_of(state, &moved.project)
        .and_then(|held| scope_items::default_workspace(&held.store, (&moved.project, "")));
    home.map_or_default(|ws| ws.id.clone())
}

fn label(
    container: Option<&Row>,
    home: &str,
) -> String {
    let Some(row) = container else {
        return home.to_owned();
    };
    let branch = row
        .workspace
        .as_ref()
        .filter(|_| row.kind == RowKind::Branch);
    branch.map_or_else(|| row.label.clone(), |ws| ws.branch.clone())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_container_is_named_by_its_branch_its_label_or_the_home() {
        let folder = Row::new("f", RowKind::Folder, 0, "Stuff");
        assert_eq!(label(Some(&folder), "Demo"), "Stuff");
        assert_eq!(label(None, "Demo"), "Demo");
    }
}
