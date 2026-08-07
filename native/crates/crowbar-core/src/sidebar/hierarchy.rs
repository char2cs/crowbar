//! Flat workspace rows nested into a parent→child tree.
//!
//! Ported from `web/src/components/layout/workspace-tree-utils.ts`.
//!
//! # Cycles are promoted to roots, not dropped
//!
//! `parent_id` comes off the wire and nothing upstream proves it is acyclic —
//! a reparent that raced with another, or a restored record, can produce
//! `a -> b -> a`. Three outcomes are possible and only one is acceptable:
//! recursing until the stack dies, silently dropping the rows (a workspace
//! that exists on disk and cannot be seen or deleted), or **promoting the row
//! to a root** so it stays reachable and the user can fix it. The TS chose the
//! third and this port keeps it.
//!
//! The walk guards on two conditions, and both are needed: reaching the node
//! itself proves the edge would close a loop through it, and revisiting any
//! already-seen ancestor proves the *existing* chain is already looped and
//! would otherwise spin forever before ever reaching the node.

use std::collections::{HashMap, HashSet};

use super::tree::SidebarWorkspace;

/// One node of the nested workspace tree.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Node {
    /// The row this node draws.
    pub workspace: SidebarWorkspace,
    /// Rows nested under it, in the order they arrived.
    pub children: Vec<Node>,
}

/// Nest a repo's flat workspace list into roots and children. Mirrors
/// `buildWorkspaceTree`.
///
/// Order is preserved: roots appear in input order, and each node's children
/// appear in input order. A row whose `parent_id` names no known workspace is
/// a root, as is one whose parent chain would close a cycle.
#[must_use]
pub fn build_workspace_tree(workspaces: &[SidebarWorkspace]) -> Vec<Node> {
    let index: HashMap<&str, usize> = workspaces
        .iter()
        .enumerate()
        .map(|(i, ws)| (ws.id.as_str(), i))
        .collect();

    // Which parent each row attaches to, or `None` for a root. Resolved for
    // every row *before* anything is built, because building bottom-up would
    // need the children of a node that does not exist yet.
    let mut parent_of: Vec<Option<usize>> = Vec::with_capacity(workspaces.len());
    for (i, ws) in workspaces.iter().enumerate() {
        parent_of.push(resolve_parent(workspaces, &index, i, ws));
    }

    // Children per row, in input order.
    let mut children_of: Vec<Vec<usize>> = vec![Vec::new(); workspaces.len()];
    let mut roots: Vec<usize> = Vec::new();
    for (i, parent) in parent_of.iter().enumerate() {
        match parent {
            Some(p) => children_of[*p].push(i),
            None => roots.push(i),
        }
    }

    roots
        .into_iter()
        .map(|i| build_node(workspaces, &children_of, i))
        .collect()
}

/// The parent index row `i` attaches to, or `None` if it is a root.
fn resolve_parent(
    workspaces: &[SidebarWorkspace],
    index: &HashMap<&str, usize>,
    i: usize,
    ws: &SidebarWorkspace,
) -> Option<usize> {
    let parent = ws
        .parent_id
        .as_deref()
        .and_then(|id| index.get(id))
        .copied();
    // A row parented to itself is a root: `parent === node` in the TS.
    let parent = parent.filter(|p| *p != i)?;
    if closes_cycle(workspaces, index, i, parent) {
        return None;
    }
    Some(parent)
}

/// Whether attaching row `i` under `parent` would close a loop — either
/// through `i` itself, or through a chain that is already looped.
fn closes_cycle(
    workspaces: &[SidebarWorkspace],
    index: &HashMap<&str, usize>,
    i: usize,
    parent: usize,
) -> bool {
    let mut seen: HashSet<usize> = HashSet::new();
    let mut cursor = Some(parent);
    while let Some(at) = cursor {
        if at == i || !seen.insert(at) {
            return true;
        }
        cursor = workspaces[at]
            .parent_id
            .as_deref()
            .and_then(|id| index.get(id))
            .copied();
    }
    false
}

/// Materialise row `i` and everything under it.
fn build_node(workspaces: &[SidebarWorkspace], children_of: &[Vec<usize>], i: usize) -> Node {
    Node {
        workspace: workspaces[i].clone(),
        children: children_of[i]
            .iter()
            .map(|c| build_node(workspaces, children_of, *c))
            .collect(),
    }
}

/// Every workspace id in `nodes` and their descendants, depth-first.
///
/// The tree is the only place the nesting is known, so "this row and
/// everything under it" — what a delete confirmation counts and what a
/// collapse hides — is answered here rather than re-walked by each caller.
#[must_use]
pub fn descendant_ids(nodes: &[Node]) -> Vec<String> {
    let mut out = Vec::new();
    collect_ids(nodes, &mut out);
    out
}

fn collect_ids(nodes: &[Node], out: &mut Vec<String>) {
    for node in nodes {
        out.push(node.workspace.id.clone());
        collect_ids(&node.children, out);
    }
}

#[cfg(test)]
mod tests {
    use super::{Node, build_workspace_tree, descendant_ids};
    use crate::sidebar::fixtures::{child_of, sidebar_workspace};
    use crate::sidebar::tree::SidebarWorkspace;

    fn ids(nodes: &[Node]) -> Vec<&str> {
        nodes.iter().map(|n| n.workspace.id.as_str()).collect()
    }

    #[test]
    fn a_flat_list_is_all_roots() {
        let rows = vec![sidebar_workspace("a"), sidebar_workspace("b")];
        let tree = build_workspace_tree(&rows);
        assert_eq!(ids(&tree), ["a", "b"]);
        assert!(tree.iter().all(|n| n.children.is_empty()));
    }

    #[test]
    fn nests_children_under_their_parent() {
        let rows = vec![
            sidebar_workspace("a"),
            child_of("b", "a"),
            child_of("c", "b"),
        ];
        let tree = build_workspace_tree(&rows);
        assert_eq!(ids(&tree), ["a"]);
        assert_eq!(ids(&tree[0].children), ["b"]);
        assert_eq!(ids(&tree[0].children[0].children), ["c"]);
    }

    #[test]
    fn input_order_is_preserved_among_siblings() {
        let rows = vec![
            sidebar_workspace("root"),
            child_of("second", "root"),
            child_of("first", "root"),
        ];
        let tree = build_workspace_tree(&rows);
        assert_eq!(ids(&tree[0].children), ["second", "first"]);
    }

    /// A parent id naming nothing is not a reason to hide a row.
    #[test]
    fn an_unknown_parent_makes_the_row_a_root() {
        let rows = vec![child_of("orphan", "gone")];
        assert_eq!(ids(&build_workspace_tree(&rows)), ["orphan"]);
    }

    #[test]
    fn a_self_parented_row_is_a_root() {
        let rows = vec![child_of("a", "a")];
        let tree = build_workspace_tree(&rows);
        assert_eq!(ids(&tree), ["a"]);
        assert!(tree[0].children.is_empty());
    }

    /// `a -> b -> a`. Both rows must stay reachable: dropping them would leave
    /// workspaces that exist on disk and cannot be seen or deleted.
    #[test]
    fn a_two_cycle_promotes_both_rows_to_roots() {
        let rows = vec![child_of("a", "b"), child_of("b", "a")];
        let tree = build_workspace_tree(&rows);
        assert_eq!(ids(&tree), ["a", "b"]);
        assert!(tree.iter().all(|n| n.children.is_empty()));
    }

    #[test]
    fn a_longer_cycle_terminates_and_keeps_every_row() {
        let rows = vec![child_of("a", "c"), child_of("b", "a"), child_of("c", "b")];
        let tree = build_workspace_tree(&rows);
        let mut all = descendant_ids(&tree);
        all.sort();
        assert_eq!(all, ["a", "b", "c"], "no row is lost to the cycle");
    }

    /// The second guard: a row hanging off an already-looped chain. Without
    /// the visited set the climb never terminates.
    #[test]
    fn a_row_attached_to_a_looped_chain_terminates() {
        let rows = vec![
            child_of("a", "b"),
            child_of("b", "a"),
            child_of("hanging", "a"),
        ];
        let tree = build_workspace_tree(&rows);
        let mut all = descendant_ids(&tree);
        all.sort();
        assert_eq!(all, ["a", "b", "hanging"]);
    }

    #[test]
    fn an_empty_parent_id_is_a_root() {
        let rows = vec![SidebarWorkspace {
            parent_id: Some(String::new()),
            ..sidebar_workspace("a")
        }];
        assert_eq!(ids(&build_workspace_tree(&rows)), ["a"]);
    }

    #[test]
    fn descendant_ids_walks_depth_first() {
        let rows = vec![
            sidebar_workspace("a"),
            child_of("a1", "a"),
            child_of("a1a", "a1"),
            child_of("a2", "a"),
            sidebar_workspace("b"),
        ];
        let tree = build_workspace_tree(&rows);
        assert_eq!(descendant_ids(&tree), ["a", "a1", "a1a", "a2", "b"]);
    }

    #[test]
    fn an_empty_list_is_an_empty_tree() {
        assert!(build_workspace_tree(&[]).is_empty());
        assert!(descendant_ids(&[]).is_empty());
    }
}
