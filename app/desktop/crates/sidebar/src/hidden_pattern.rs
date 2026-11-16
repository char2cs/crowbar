//! Gitignore-style patterns of the hidden file and directory settings.

use crate::file_kind::FileKind;

pub(crate) fn matches(
    pattern: &str,
    path: &str,
    kind: FileKind,
) -> bool {
    let is_dir = matches!(kind, FileKind::Directory(_));
    let pattern = pattern.trim();
    let is_inert = pattern.is_empty() || pattern.starts_with(['#', '!']);
    if is_inert {
        return false;
    }
    let (body, dir_only) = match pattern.strip_suffix('/') {
        Some(body) => (body, true),
        None => (pattern, false),
    };
    if dir_only && !is_dir {
        return false;
    }
    let body = body.strip_prefix("**/").unwrap_or(body);
    let target = target_of(body, path);
    let body = body.trim_start_matches('/');
    wild(&chars(body), &chars(target))
}

fn target_of<'a>(
    body: &str,
    path: &'a str,
) -> &'a str {
    if body.contains('/') {
        return path;
    }
    path.rsplit('/').next().unwrap_or(path)
}

fn chars(text: &str) -> Vec<char> {
    text.chars().collect()
}

fn wild(
    pattern: &[char],
    text: &[char],
) -> bool {
    let Some((first, rest)) = pattern.split_first() else {
        return text.is_empty();
    };
    match first {
        '*' => star(rest, text),
        '?' => text
            .split_first()
            .is_some_and(|(next, tail)| *next != '/' && wild(rest, tail)),
        literal => text
            .split_first()
            .is_some_and(|(next, tail)| next == literal && wild(rest, tail)),
    }
}

fn star(
    rest: &[char],
    text: &[char],
) -> bool {
    let (deep, rest) = match rest.split_first() {
        Some(('*', tail)) => (true, tail),
        _ => (false, rest),
    };
    (0..=text.len()).any(|taken| {
        text.split_at_checked(taken)
            .is_some_and(|(head, tail)| (deep || !head.contains(&'/')) && wild(rest, tail))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::expansion::Expansion;

    #[test]
    fn patterns_follow_the_gitignore_rules_the_settings_describe() {
        const DIR: FileKind = FileKind::Directory(Expansion::Expanded);
        const FILE: FileKind = FileKind::File;
        let cases = [
            ("*.log", "a/b/run.log", FILE, true),
            ("*.log", "a/b/run.txt", FILE, false),
            ("node_modules/", "web/node_modules", DIR, true),
            ("node_modules/", "web/node_modules", FILE, false),
            ("build", "build", DIR, true),
            ("src/*.rs", "src/main.rs", FILE, true),
            ("src/*.rs", "src/deep/main.rs", FILE, false),
            ("src/**/gen.rs", "src/a/b/gen.rs", FILE, true),
            ("**/dist", "pkg/dist", DIR, true),
            ("/docs", "docs", DIR, true),
            ("file?.md", "file1.md", FILE, true),
            ("# note", "# note", FILE, false),
            ("", "a", FILE, false),
            ("!keep", "keep", FILE, false),
        ];
        for (pattern, path, kind, expected) in cases {
            assert_eq!(matches(pattern, path, kind), expected, "{pattern} {path}");
        }
    }
}
