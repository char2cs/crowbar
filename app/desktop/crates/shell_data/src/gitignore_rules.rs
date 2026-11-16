//! Which paths of a workspace its `.gitignore` files hide.

use ignore::Match;
use ignore::gitignore::{Gitignore, GitignoreBuilder};

use crate::entry_kind::EntryKind;

const GIT_DIRECTORY: &str = ".git";

#[derive(Debug, Clone)]
struct RuleSet {
    directory: String,
    matcher: Gitignore,
}

/// The loaded `.gitignore` files, each owning the directory it sits in.
#[derive(Debug, Clone, Default)]
pub(crate) struct IgnoreRules {
    sets: Vec<RuleSet>,
}

fn depth_of(directory: &str) -> usize {
    directory.split('/').filter(|part| !part.is_empty()).count()
}

fn relative_to<'a>(
    path: &'a str,
    directory: &str,
) -> Option<&'a str> {
    if directory.is_empty() {
        return Some(path);
    }
    path.strip_prefix(directory)?.strip_prefix('/')
}

fn matcher_of(text: &str) -> Gitignore {
    let mut builder = GitignoreBuilder::new("");
    for line in text.lines() {
        if let Err(error) = builder.add_line(None, line) {
            tracing::debug!(%error, line, "a malformed ignore pattern is skipped");
        }
    }
    builder.build().unwrap_or_else(|_| Gitignore::empty())
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
        self.sets.retain(|set| set.directory != directory);
        self.sets.push(RuleSet {
            directory: directory.to_owned(),
            matcher: matcher_of(text),
        });
        self.sets
            .sort_by_key(|set| (depth_of(&set.directory), set.directory.clone()));
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
            .filter_map(|set| verdict(set, path, kind))
            .next_back()
            .unwrap_or(false)
    }
}

fn verdict(
    set: &RuleSet,
    path: &str,
    kind: EntryKind,
) -> Option<bool> {
    let relative = relative_to(path, &set.directory)?;
    match set.matcher.matched(relative, kind == EntryKind::Folder) {
        Match::Ignore(_) => Some(true),
        Match::Whitelist(_) => Some(false),
        Match::None => None,
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

    /// A trailing slash on the path marks a directory.
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

    #[test]
    fn the_rules_hide_what_git_would() {
        for (name, files, path, hidden) in CASES {
            let kind = if path.ends_with('/') {
                EntryKind::Folder
            } else {
                EntryKind::File
            };
            let bare = path.trim_end_matches('/');
            assert_eq!(rules_of(files).ignores(bare, kind), hidden, "{name}");
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
