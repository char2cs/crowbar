//! Placing folders, workspaces and chats into one tree, as the daemon's placement rules read.

use std::collections::{BTreeMap, HashMap, HashSet};

use crate::created_key::created_key;
use crate::forest_item::ForestItem;
use crate::forest_tree::ForestTree;
use crate::item_kind::ItemKind;

type Parents = HashMap<String, Option<String>>;

pub(crate) fn build(items: &[ForestItem]) -> Vec<ForestTree> {
    let by_id: HashMap<&str, &ForestItem> = items.iter().map(|i| (i.id.as_str(), i)).collect();
    let mut parents = Parents::new();
    for item in items {
        parents.insert(item.id.clone(), parent_of(item, &by_id, &parents));
    }
    let children = items.iter().fold(
        BTreeMap::<String, Vec<&ForestItem>>::new(),
        |mut children, item| {
            let parent = parents.get(&item.id).cloned().flatten();
            let rooted = closes_cycle(&item.id, &parents);
            children
                .entry(parent.filter(|_| !rooted).unwrap_or_default())
                .or_default()
                .push(item);
            children
        },
    );
    level("", &children)
}

fn parent_of(
    item: &ForestItem,
    by_id: &HashMap<&str, &ForestItem>,
    parents: &Parents,
) -> Option<String> {
    let exists = |id: &str| {
        (!id.is_empty() && id != item.id && by_id.contains_key(id)).then(|| id.to_owned())
    };
    match item.kind {
        ItemKind::Folder => exists(&item.filed),
        ItemKind::Chat => exists(&item.filed).or_else(|| exists(&item.ground)),
        ItemKind::Workspace => workspace_parent(
            (&exists(&item.filed), &exists(&item.ground)),
            (by_id, parents),
        ),
    }
}

fn workspace_parent(
    (folder, fork): (&Option<String>, &Option<String>),
    (by_id, parents): (&HashMap<&str, &ForestItem>, &Parents),
) -> Option<String> {
    let Some(folder) = folder else {
        return fork.clone();
    };
    let anchor = anchor_of(folder, by_id, parents);
    let keeps_chain = fork
        .as_ref()
        .is_none_or(|fork| anchor.as_deref() == Some(fork.as_str()));
    keeps_chain.then(|| folder.clone()).or_else(|| fork.clone())
}

fn anchor_of(
    id: &str,
    by_id: &HashMap<&str, &ForestItem>,
    parents: &Parents,
) -> Option<String> {
    let (chain, _) = ancestry(id, parents);
    chain.into_iter().find(|member| {
        by_id
            .get(member.as_str())
            .is_some_and(|item| item.kind == ItemKind::Workspace)
    })
}

fn ancestry(
    id: &str,
    parents: &Parents,
) -> (Vec<String>, bool) {
    let mut chain = vec![id.to_owned()];
    let first = parents.get(id).cloned().flatten();
    let above = std::iter::successors(first, |at| parents.get(at).cloned().flatten());
    let repeated = above.into_iter().find(|at| {
        let seen = chain.contains(at);
        chain.push(at.clone());
        seen
    });
    (chain, repeated.is_some())
}

fn closes_cycle(
    id: &str,
    parents: &Parents,
) -> bool {
    ancestry(id, parents).1
}

fn level(
    parent: &str,
    children: &BTreeMap<String, Vec<&ForestItem>>,
) -> Vec<ForestTree> {
    let mut members: Vec<&ForestItem> = children.get(parent).cloned().unwrap_or_default();
    members.sort_by_cached_key(|item| (item.order, item.kind, created_key(&item.created)));
    members
        .into_iter()
        .map(|item| ForestTree {
            id: item.id.clone(),
            kind: item.kind,
            children: level(&item.id, children),
        })
        .collect()
}

pub(crate) fn fold_owners(
    roots: Vec<ForestTree>,
    owners: &HashMap<String, String>,
) -> Vec<ForestTree> {
    let owner_ids: HashSet<&str> = owners.values().map(String::as_str).collect();
    let mut detached = HashMap::new();
    let kept = strip(roots, &owner_ids, &mut detached);
    merge(kept, owners, &detached)
}

fn strip(
    level: Vec<ForestTree>,
    owner_ids: &HashSet<&str>,
    detached: &mut HashMap<String, ForestTree>,
) -> Vec<ForestTree> {
    let (owners, kept): (Vec<ForestTree>, Vec<ForestTree>) = level
        .into_iter()
        .map(|node| ForestTree {
            children: strip(node.children, owner_ids, detached),
            ..node
        })
        .partition(|node| node.kind == ItemKind::Chat && owner_ids.contains(node.id.as_str()));
    detached.extend(owners.into_iter().map(|node| (node.id.clone(), node)));
    kept
}

fn merge(
    level: Vec<ForestTree>,
    owners: &HashMap<String, String>,
    detached: &HashMap<String, ForestTree>,
) -> Vec<ForestTree> {
    level
        .into_iter()
        .map(|node| fold(node, owners, detached))
        .collect()
}

fn fold(
    mut node: ForestTree,
    owners: &HashMap<String, String>,
    detached: &HashMap<String, ForestTree>,
) -> ForestTree {
    node.children = merge(node.children, owners, detached);
    let owner = (node.kind == ItemKind::Workspace)
        .then(|| owners.get(&node.id).and_then(|chat| detached.get(chat)))
        .flatten();
    let adopted = owner.map_or_default(|owner| merge(owner.children.clone(), owners, detached));
    node.children.extend(adopted);
    node
}

#[cfg(test)]
mod tests {
    use super::*;

    fn item(
        id: &str,
        kind: ItemKind,
        (filed, ground): (&str, &str),
    ) -> ForestItem {
        ForestItem {
            id: id.into(),
            kind,
            order: 0,
            created: String::new(),
            filed: filed.into(),
            ground: ground.into(),
        }
    }

    fn ids(trees: &[ForestTree]) -> Vec<String> {
        trees
            .iter()
            .map(|tree| format!("{}[{}]", tree.id, ids(&tree.children).join(",")))
            .collect()
    }

    use ItemKind::{Chat, Folder, Workspace};

    type Spec = (
        &'static str,
        ItemKind,
        (&'static str, &'static str),
        (i64, &'static str),
    );

    const fn spec(
        id: &'static str,
        kind: ItemKind,
        parents: (&'static str, &'static str),
    ) -> Spec {
        (id, kind, parents, (0, ""))
    }

    fn items(specs: &[Spec]) -> Vec<ForestItem> {
        specs
            .iter()
            .map(|(id, kind, parents, (order, created))| ForestItem {
                order: *order,
                created: (*created).to_owned(),
                ..item(id, *kind, *parents)
            })
            .collect()
    }

    const OWNER_OF_W: Option<(&str, &str)> = Some(("w", "owner"));

    type Scenario = (
        &'static str,
        &'static [Spec],
        Option<(&'static str, &'static str)>,
        &'static [&'static str],
    );

    const CASES: [Scenario; 6] = [
        (
            "chats sit under their parent or their workspace",
            &[
                spec("w", Workspace, ("", "")),
                spec("thread", Chat, ("owner", "w")),
                spec("owner", Chat, ("", "w")),
                spec("loose", Chat, ("missing", "")),
            ],
            None,
            &["w[owner[thread[]]]", "loose[]"],
        ),
        (
            "a folder wins for a workspace without a fork parent",
            &[spec("f", Folder, ("", "")), spec("w", Workspace, ("f", ""))],
            None,
            &["f[w[]]"],
        ),
        (
            "a folder that would split a fork chain loses to the fork parent",
            &[
                spec("a", Workspace, ("", "")),
                spec("f", Folder, ("", "")),
                spec("b", Workspace, ("f", "a")),
            ],
            None,
            &["f[]", "a[b[]]"],
        ),
        (
            "a cycle roots its members",
            &[spec("x", Folder, ("y", "")), spec("y", Folder, ("x", ""))],
            None,
            &["x[]", "y[]"],
        ),
        (
            "siblings sort by order then kind then creation",
            &[
                ("late", Chat, ("", ""), (0, "2026-01-02T00:00:00Z")),
                ("early", Chat, ("", ""), (0, "2026-01-01T00:00:00Z")),
                spec("folder", Folder, ("", "")),
                ("first", Chat, ("", ""), (-1, "")),
            ],
            None,
            &["first[]", "folder[]", "early[]", "late[]"],
        ),
        (
            "an owner chat folds into its workspace",
            &[
                spec("w", Workspace, ("", "")),
                spec("owner", Chat, ("", "w")),
                spec("thread", Chat, ("owner", "w")),
            ],
            OWNER_OF_W,
            &["w[thread[]]"],
        ),
    ];

    #[test]
    fn each_arrangement_of_items_places_into_its_tree() {
        for (name, specs, owner, expected) in CASES {
            let placed = build(&items(specs));
            let owners: HashMap<String, String> = owner
                .map(|(workspace, chat)| (workspace.to_owned(), chat.to_owned()))
                .into_iter()
                .collect();
            let folded = fold_owners(placed, &owners);
            assert_eq!(ids(&folded), expected, "{name}");
        }
    }
}
