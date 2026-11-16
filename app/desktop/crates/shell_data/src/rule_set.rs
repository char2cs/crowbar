//! The patterns of one `.gitignore` file and the directory they rule.

use ignore::Match;
use ignore::gitignore::{Gitignore, GitignoreBuilder};

use crate::entry_kind::EntryKind;

#[derive(Debug, Clone)]
pub(crate) struct RuleSet {
    directory: String,
    matcher: Gitignore,
}

impl RuleSet {
    pub(crate) fn parse(
        directory: &str,
        text: &str,
    ) -> Self {
        Self {
            directory: directory.to_owned(),
            matcher: matcher_of(text),
        }
    }

    pub(crate) fn directory(&self) -> &str {
        &self.directory
    }

    pub(crate) fn order(&self) -> (usize, String) {
        (depth_of(&self.directory), self.directory.clone())
    }

    pub(crate) fn verdict(
        &self,
        path: &str,
        kind: EntryKind,
    ) -> Option<bool> {
        let relative = relative_to(path, &self.directory)?;
        match self.matcher.matched(relative, kind == EntryKind::Folder) {
            Match::Ignore(_) => Some(true),
            Match::Whitelist(_) => Some(false),
            Match::None => None,
        }
    }
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
    text.lines().for_each(|line| add_line(&mut builder, line));
    builder.build().unwrap_or_else(|_| Gitignore::empty())
}

fn add_line(
    builder: &mut GitignoreBuilder,
    line: &str,
) {
    if let Err(error) = builder.add_line(None, line) {
        tracing::debug!(%error, line, "a malformed ignore pattern is skipped");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_rule_set_rules_only_paths_under_its_directory() {
        type Case = (&'static str, &'static str, &'static str, Option<bool>);
        const CASES: &[Case] = &[
            ("", "*.log", "a/x.log", Some(true)),
            ("", "!keep.log", "keep.log", Some(false)),
            ("src", "gen.rs", "src/gen.rs", Some(true)),
            ("src", "gen.rs", "lib/gen.rs", None),
            ("src", "gen.rs", "srcs/gen.rs", None),
            ("", "*.log", "main.rs", None),
        ];
        for (directory, text, path, expected) in CASES {
            let rules = RuleSet::parse(directory, text);
            assert_eq!(rules.verdict(path, EntryKind::File), *expected, "{path}");
        }
    }

    #[test]
    fn shallower_directories_sort_first() {
        const CASES: &[(&str, usize)] = &[("", 0), ("src", 1), ("src/a/b", 3)];
        for (directory, depth) in CASES {
            let order = RuleSet::parse(directory, "").order();
            assert_eq!(order, (*depth, (*directory).to_owned()));
        }
    }
}
