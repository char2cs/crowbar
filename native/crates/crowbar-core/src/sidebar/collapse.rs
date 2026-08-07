//! Which repo sections and workspace subtrees are collapsed.
//!
//! Ported from `web/src/lib/store/sidebar.ts`'s `collapsedRepos` /
//! `collapsedWorkspaces`.
//!
//! # Collapsed is the exception, so the set holds the collapsed ones
//!
//! Both sets are *negative*: membership means collapsed, absence means
//! expanded. That is what makes a newly-streamed repo appear expanded without
//! anything having to add it — the alternative, a set of expanded ids, would
//! need every arriving row registered before it could be seen, and a dropped
//! registration would hide real data.

use std::collections::HashSet;

/// The collapsed-state of the tree: which repo sections and which workspace
/// subtrees are folded shut.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Collapsed {
    repos: HashSet<String>,
    workspaces: HashSet<String>,
}

impl Collapsed {
    /// Nothing collapsed.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Whether a repo section is folded shut.
    #[must_use]
    pub fn is_repo_collapsed(&self, repo_id: &str) -> bool {
        self.repos.contains(repo_id)
    }

    /// Whether a workspace's children are hidden.
    #[must_use]
    pub fn is_workspace_collapsed(&self, ws_id: &str) -> bool {
        self.workspaces.contains(ws_id)
    }

    /// Fold a repo section, or unfold it. Returns the new state.
    pub fn toggle_repo(&mut self, repo_id: &str) -> bool {
        toggle(&mut self.repos, repo_id)
    }

    /// Fold a workspace's children, or unfold them. Returns the new state.
    pub fn toggle_workspace(&mut self, ws_id: &str) -> bool {
        toggle(&mut self.workspaces, ws_id)
    }

    /// Force a workspace open — used when a row has to be revealed, e.g. a
    /// newly created child under a collapsed parent, which would otherwise be
    /// created into a subtree the user cannot see.
    pub fn expand_workspace(&mut self, ws_id: &str) {
        self.workspaces.remove(ws_id);
    }

    /// Force a repo section open, for the same reason.
    pub fn expand_repo(&mut self, repo_id: &str) {
        self.repos.remove(repo_id);
    }

    /// Drop ids that no longer exist, so a deleted-then-recreated id does not
    /// inherit the old row's collapsed state.
    ///
    /// This runs on every tree rebuild, so it takes the live ids as sets the
    /// caller already has rather than rebuilding them per call.
    pub fn retain_known(&mut self, repo_ids: &HashSet<String>, ws_ids: &HashSet<String>) {
        self.repos.retain(|id| repo_ids.contains(id));
        self.workspaces.retain(|id| ws_ids.contains(id));
    }

    /// How many rows are folded — repos and workspaces together. Exists so
    /// tests can assert the pruning above actually removed something rather
    /// than asserting on a private field.
    #[must_use]
    pub fn len(&self) -> usize {
        self.repos.len() + self.workspaces.len()
    }

    /// Whether nothing at all is collapsed.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.repos.is_empty() && self.workspaces.is_empty()
    }
}

/// Insert or remove `id`, returning whether it is now present.
fn toggle(set: &mut HashSet<String>, id: &str) -> bool {
    if set.remove(id) {
        false
    } else {
        set.insert(id.to_owned());
        true
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashSet;

    use super::Collapsed;

    fn set(ids: &[&str]) -> HashSet<String> {
        ids.iter().map(|s| (*s).to_string()).collect()
    }

    /// The sets are negative: absence means expanded, so a repo nobody has
    /// registered still renders open.
    #[test]
    fn everything_is_expanded_by_default() {
        let collapsed = Collapsed::new();
        assert!(!collapsed.is_repo_collapsed("r1"));
        assert!(!collapsed.is_workspace_collapsed("w1"));
        assert!(collapsed.is_empty());
        assert_eq!(collapsed.len(), 0);
    }

    #[test]
    fn toggling_a_repo_reports_and_flips_its_state() {
        let mut collapsed = Collapsed::new();
        assert!(collapsed.toggle_repo("r1"));
        assert!(collapsed.is_repo_collapsed("r1"));
        assert!(!collapsed.toggle_repo("r1"));
        assert!(!collapsed.is_repo_collapsed("r1"));
    }

    #[test]
    fn toggling_a_workspace_reports_and_flips_its_state() {
        let mut collapsed = Collapsed::new();
        assert!(collapsed.toggle_workspace("w1"));
        assert!(collapsed.is_workspace_collapsed("w1"));
        assert!(!collapsed.toggle_workspace("w1"));
        assert!(!collapsed.is_workspace_collapsed("w1"));
    }

    #[test]
    fn the_two_id_spaces_are_independent() {
        let mut collapsed = Collapsed::new();
        collapsed.toggle_repo("x");
        assert!(collapsed.is_repo_collapsed("x"));
        assert!(
            !collapsed.is_workspace_collapsed("x"),
            "a repo and a workspace may share an id without sharing a state"
        );
    }

    /// A child created under a collapsed parent would otherwise land in a
    /// subtree the user cannot see.
    #[test]
    fn a_row_can_be_forced_open() {
        let mut collapsed = Collapsed::new();
        collapsed.toggle_workspace("w1");
        collapsed.expand_workspace("w1");
        assert!(!collapsed.is_workspace_collapsed("w1"));

        collapsed.toggle_repo("r1");
        collapsed.expand_repo("r1");
        assert!(!collapsed.is_repo_collapsed("r1"));
    }

    #[test]
    fn expanding_something_already_open_is_a_no_op() {
        let mut collapsed = Collapsed::new();
        collapsed.expand_workspace("never-seen");
        collapsed.expand_repo("never-seen");
        assert!(collapsed.is_empty());
    }

    /// A deleted-then-recreated id must not inherit the old row's state.
    #[test]
    fn pruning_drops_ids_that_no_longer_exist() {
        let mut collapsed = Collapsed::new();
        collapsed.toggle_repo("r-live");
        collapsed.toggle_repo("r-gone");
        collapsed.toggle_workspace("w-live");
        collapsed.toggle_workspace("w-gone");
        assert_eq!(collapsed.len(), 4);

        collapsed.retain_known(&set(&["r-live"]), &set(&["w-live"]));

        assert_eq!(collapsed.len(), 2);
        assert!(collapsed.is_repo_collapsed("r-live"));
        assert!(!collapsed.is_repo_collapsed("r-gone"));
        assert!(collapsed.is_workspace_collapsed("w-live"));
        assert!(!collapsed.is_workspace_collapsed("w-gone"));
    }
}
