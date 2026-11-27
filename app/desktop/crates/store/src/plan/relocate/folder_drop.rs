use daemonapi::{Call, Route};
use serde_json::{Map, Value, json};
use sidebar::{Row, RowKind};

use crate::live::{PlanError, State};
use crate::plan::Plan;
use crate::plan::relocate::slot::Slot;
use crate::reads::host_of;
use crate::rows::target::{Target, resolve};

pub(super) fn plan(
    state: &State,
    (rows, moved): (&[Row], &Target),
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let held = host_of(state, &moved.project).ok_or(PlanError::SpaceNotLoaded)?;
    let mut body = Map::new();
    body.insert("order".into(), json!(slot.index));
    if slot.parent != slot.from {
        let parent = parent_id(state, (rows, moved), &slot.parent);
        body.insert("parentId".into(), Value::String(parent));
    }
    let folders = Route::folders(&moved.project, &moved.repo);
    let path = format!("{folders}/{}", moved.id);
    Ok(Plan::of(
        &held.spec.id,
        vec![Call::patch(path).json(Value::Object(body))],
    ))
}

fn parent_id(
    state: &State,
    (rows, moved): (&[Row], &Target),
    container: &str,
) -> String {
    let Some(row) = rows.iter().find(|row| row.id.as_str() == container) else {
        return String::new();
    };
    match row.kind {
        RowKind::Folder => container.to_owned(),
        RowKind::Chat if moved.repo.is_empty() => container.to_owned(),
        RowKind::Branch => resolve(state, container)
            .and_then(|target| target.workspace().map(str::to_owned))
            .unwrap_or_default(),
        RowKind::Chat | RowKind::RepoHeader => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::ProjectId;

    use super::*;
    use crate::reads::tree;
    use crate::rows::target::Kind;
    use crate::test_support::state;

    #[test]
    fn a_folder_filed_into_a_branch_names_its_workspace() {
        let (state, _) = state();
        let rows = tree(&state, &ProjectId::from("p")).rows;
        let moved = Target {
            id: "f".into(),
            project: "p".into(),
            repo: "r".into(),
            kind: Kind::Folder,
        };
        assert_eq!(parent_id(&state, (&rows, &moved), "bo"), "b");
        assert_eq!(parent_id(&state, (&rows, &moved), "rdo"), "");
    }
}
