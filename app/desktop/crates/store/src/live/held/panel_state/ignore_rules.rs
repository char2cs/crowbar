use daemonapi::EntryKind;

use crate::live::held::panel_state::rule_set::RuleSet;

const GIT_DIRECTORY: &str = ".git";

#[derive(Debug, Clone, Default)]
pub(crate) struct IgnoreRules {
    sets: Vec<RuleSet>,
}

fn ancestors(path: &str) -> impl Iterator<Item = &str> {
    path.match_indices('/').filter_map(|(at, _)| path.get(..at))
}

impl IgnoreRules {
    pub(crate) fn learn(
        &mut self,
        directory: &str,
        text: &str,
    ) {
        self.sets.retain(|set| set.directory() != directory);
        self.sets.push(RuleSet::parse(directory, text));
        self.sets.sort_by_key(RuleSet::order);
    }

    pub(crate) fn ignores(
        &self,
        path: &str,
        kind: EntryKind,
    ) -> bool {
        if path == GIT_DIRECTORY {
            return false;
        }
        ancestors(path).any(|parent| self.decided(parent, EntryKind::Folder))
            || self.decided(path, kind)
    }

    fn decided(
        &self,
        path: &str,
        kind: EntryKind,
    ) -> bool {
        self.sets
            .iter()
            .filter_map(|set| set.verdict(path, kind))
            .next_back()
            .unwrap_or(false)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Files = &'static [(&'static str, &'static str)];

    const ROOT_LOG: Files = &[("", "*.log")];
    const ROOT_BUILD: Files = &[("", "build/")];
    const ROOT_ANCHORED: Files = &[("", "/out.txt")];
    const NEGATED: Files = &[("", "*.log\n!keep.log")];
    const TARGET: Files = &[("", "target/")];
    const NESTED: Files = &[("src", "gen.rs")];
    const NESTED_NEGATION: Files = &[("", "*.tmp"), ("src", "!a.tmp")];
    const COMMENTS: Files = &[("", "# note\n\nsecret")];
    const DOTS: Files = &[("", ".*")];

    type Case = (&'static str, Files, &'static str, bool);

    const CASES: [Case; 14] = [
        ("a glob hides matching files", ROOT_LOG, "debug.log", true),
        ("a glob leaves other files", ROOT_LOG, "main.rs", false),
        (
            "a glob matches in any directory",
            ROOT_LOG,
            "a/b/x.log",
            true,
        ),
        (
            "a trailing slash hides directories",
            ROOT_BUILD,
            "build/",
            true,
        ),
        ("a trailing slash spares a file", ROOT_BUILD, "build", false),
        (
            "a leading slash anchors to the root",
            ROOT_ANCHORED,
            "out.txt",
            true,
        ),
        (
            "an anchored pattern misses deeper files",
            ROOT_ANCHORED,
            "a/out.txt",
            false,
        ),
        ("a negation brings a file back", NEGATED, "keep.log", false),
        (
            "a hidden parent hides what is inside",
            TARGET,
            "target/debug/x",
            true,
        ),
        (
            "a nested file rules its own directory",
            NESTED,
            "src/gen.rs",
            true,
        ),
        (
            "a nested file spares a sibling",
            NESTED,
            "lib/gen.rs",
            false,
        ),
        (
            "a nested negation beats the root rule",
            NESTED_NEGATION,
            "src/a.tmp",
            false,
        ),
        (
            "comments and blank lines are skipped",
            COMMENTS,
            "secret",
            true,
        ),
        ("the git directory is never hidden", DOTS, ".git/", false),
    ];

    fn rules_of(files: Files) -> IgnoreRules {
        let mut rules = IgnoreRules::default();
        for (directory, text) in files {
            rules.learn(directory, text);
        }
        rules
    }

    fn kind_of(path: &str) -> EntryKind {
        if path.ends_with('/') {
            return EntryKind::Folder;
        }
        EntryKind::File
    }

    #[test]
    fn the_rules_hide_what_git_would() {
        for (name, files, path, hidden) in CASES {
            let bare = path.trim_end_matches('/');
            let found = rules_of(files).ignores(bare, kind_of(path));
            assert_eq!(found, hidden, "{name}");
        }
    }

    #[test]
    fn learning_a_directory_again_replaces_its_rules() {
        let mut rules = rules_of(ROOT_LOG);
        rules.learn("", "*.tmp");
        assert!(!rules.ignores("a.log", EntryKind::File));
        assert!(rules.ignores("a.tmp", EntryKind::File));
    }
}
