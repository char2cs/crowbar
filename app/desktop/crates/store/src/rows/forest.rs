//! Placing folders, workspaces and chats into one tree, as the daemon's placement rules read.

mod created_key;
mod item;
mod item_kind;
mod sibling_key;
mod tree;

pub(crate) use item::Item;
pub(crate) use item_kind::ItemKind;
pub(crate) use sibling_key::SiblingKey;
pub(crate) use tree::Tree;

use std::collections::{BTreeMap, HashMap, HashSet};

type Parents = HashMap<String, Option<String>>;

pub(crate) fn build(items: &[Item]) -> Vec<Tree> {
    let by_id: HashMap<&str, &Item> = items.iter().map(|i| (i.id.as_str(), i)).collect();
    let mut parents = Parents::new();
    for item in items {
        parents.insert(item.id.clone(), parent_of(item, &by_id, &parents));
    }
    let children = items.iter().fold(
        BTreeMap::<String, Vec<&Item>>::new(),
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
    item: &Item,
    by_id: &HashMap<&str, &Item>,
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
    (by_id, parents): (&HashMap<&str, &Item>, &Parents),
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
    by_id: &HashMap<&str, &Item>,
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
    children: &BTreeMap<String, Vec<&Item>>,
) -> Vec<Tree> {
    let members: Vec<&Item> = children.get(parent).cloned().unwrap_or_default();
    let mut trees: Vec<Tree> = members
        .into_iter()
        .map(|item| Tree {
            id: item.id.clone(),
            kind: item.kind,
            key: SiblingKey::of_item(item),
            children: level(&item.id, children),
        })
        .collect();
    trees.sort_by(|a, b| a.key.cmp(&b.key));
    trees
}

pub(crate) fn fold_owners(
    roots: Vec<Tree>,
    owners: &HashMap<String, String>,
) -> Vec<Tree> {
    let owner_ids: HashSet<&str> = owners.values().map(String::as_str).collect();
    let mut detached = HashMap::new();
    let kept = strip(roots, &owner_ids, &mut detached);
    merge(kept, owners, &detached)
}

fn strip(
    level: Vec<Tree>,
    owner_ids: &HashSet<&str>,
    detached: &mut HashMap<String, Tree>,
) -> Vec<Tree> {
    let (owners, kept): (Vec<Tree>, Vec<Tree>) = level
        .into_iter()
        .map(|node| Tree {
            children: strip(node.children, owner_ids, detached),
            ..node
        })
        .partition(|node| node.kind == ItemKind::Chat && owner_ids.contains(node.id.as_str()));
    detached.extend(owners.into_iter().map(|node| (node.id.clone(), node)));
    kept
}

fn merge(
    level: Vec<Tree>,
    owners: &HashMap<String, String>,
    detached: &HashMap<String, Tree>,
) -> Vec<Tree> {
    level
        .into_iter()
        .map(|node| fold(node, owners, detached))
        .collect()
}

fn fold(
    mut node: Tree,
    owners: &HashMap<String, String>,
    detached: &HashMap<String, Tree>,
) -> Tree {
    node.children = merge(node.children, owners, detached);
    let owner = (node.kind == ItemKind::Workspace)
        .then(|| owners.get(&node.id).and_then(|chat| detached.get(chat)))
        .flatten();
    let adopted = owner.map_or_default(|owner| merge(owner.children.clone(), owners, detached));
    node.children.extend(adopted);
    node.children.sort_by(|a, b| a.key.cmp(&b.key));
    node
}

#[cfg(test)]
mod tests {
    use super::*;

    fn item(
        id: &str,
        kind: ItemKind,
        (filed, ground): (&str, &str),
    ) -> Item {
        Item {
            id: id.into(),
            kind,
            order: 0,
            created: String::new(),
            filed: filed.into(),
            ground: ground.into(),
        }
    }

    fn ids(trees: &[Tree]) -> Vec<String> {
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

    fn items(specs: &[Spec]) -> Vec<Item> {
        specs
            .iter()
            .map(|(id, kind, parents, (order, created))| Item {
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

    const CASES: [Scenario; 7] = [
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
        (
            "an owner chat's threads sort among its workspace's own children",
            &[
                spec("w", Workspace, ("", "")),
                spec("owner", Chat, ("", "w")),
                ("later", Chat, ("", "w"), (1, "")),
                spec("thread", Chat, ("owner", "w")),
            ],
            OWNER_OF_W,
            &["w[thread[],later[]]"],
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
