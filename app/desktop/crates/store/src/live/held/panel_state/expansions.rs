//! The folders open in the Files panel, per workspace, as develop's `expandedPathsByWorkspace`.

use std::collections::{HashMap, HashSet};

#[derive(Debug, Clone, Default)]
pub(crate) struct Expansions {
    open: HashMap<String, HashSet<String>>,
}

impl Expansions {
    pub(crate) fn contains(
        &self,
        workspace: &str,
        path: &str,
    ) -> bool {
        self.open
            .get(workspace)
            .is_some_and(|paths| paths.contains(path))
    }

    pub(crate) fn paths(
        &self,
        workspace: &str,
    ) -> Vec<String> {
        self.open
            .get(workspace)
            .map_or_default(|paths| paths.iter().cloned().collect())
    }

    pub(crate) fn of_mut(
        &mut self,
        workspace: &str,
    ) -> &mut HashSet<String> {
        self.open.entry(workspace.to_owned()).or_default()
    }

    pub(crate) fn toggle(
        &mut self,
        workspace: &str,
        path: &str,
    ) -> bool {
        let open = self.of_mut(workspace);
        if open.remove(path) {
            return false;
        }
        open.insert(path.to_owned());
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_toggle_alternates_within_its_workspace_only() {
        let mut expansions = Expansions::default();
        assert!(expansions.toggle("a", "src"));
        assert!(expansions.contains("a", "src"));
        assert!(!expansions.contains("b", "src"));
        assert_eq!(expansions.paths("a"), ["src"]);
        assert!(!expansions.toggle("a", "src"));
        assert_eq!(expansions.paths("a"), Vec::<String>::new());
    }
}
