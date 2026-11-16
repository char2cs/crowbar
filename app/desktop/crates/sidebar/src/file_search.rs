//! The filter box of the Files panel and the rows it keeps.

use gpui::{Entity, Modifiers, Subscription};
use ui::TextInput;

use crate::file_kind::FileKind;
use crate::file_row::FileRow;
use crate::match_direction::MatchDirection;

const OPEN_KEY: &str = "/";
const OPEN_KEY_WITH_MODIFIER: &str = "f";

#[derive(Debug)]
pub(crate) struct FileSearch {
    pub(crate) field: Entity<TextInput>,
    pub(crate) query: String,
    pub(crate) _subscription: Subscription,
}

pub(crate) fn opens_search(
    key: &str,
    modifiers: Modifiers,
) -> bool {
    if modifiers.secondary() {
        return key == OPEN_KEY_WITH_MODIFIER;
    }
    key == OPEN_KEY && !modifiers.modified()
}

fn needle(query: &str) -> String {
    query.trim().to_lowercase()
}

pub(crate) fn matching(
    rows: Vec<FileRow>,
    query: &str,
) -> Vec<FileRow> {
    let needle = needle(query);
    if needle.is_empty() {
        return rows;
    }
    let hits: Vec<String> = rows
        .iter()
        .filter(|row| row.name.to_lowercase().contains(&needle))
        .map(|row| row.path.clone())
        .collect();
    rows.into_iter().filter(|row| keeps(row, &hits)).collect()
}

pub(crate) fn highlight(
    name: &str,
    query: &str,
) -> Option<std::ops::Range<usize>> {
    let needle = needle(query);
    let lower = name.to_lowercase();
    if needle.is_empty() || lower.len() != name.len() {
        return None;
    }
    let start = lower.find(&needle)?;
    Some(start..start + needle.len())
}

pub(crate) fn step_match(
    rows: &[FileRow],
    (query, selected): (&str, Option<&str>),
    direction: MatchDirection,
) -> Option<String> {
    let hits: Vec<&FileRow> = rows
        .iter()
        .filter(|row| highlight(&row.name, query).is_some())
        .collect();
    let last = hits.len().checked_sub(1)?;
    let at = hits
        .iter()
        .position(|row| Some(row.path.as_str()) == selected);
    let next = match (at, direction) {
        (None, MatchDirection::Forward) => 0,
        (Some(index), MatchDirection::Forward) if index == last => 0,
        (Some(index), MatchDirection::Forward) => index + 1,
        (None | Some(0), MatchDirection::Backward) => last,
        (Some(index), MatchDirection::Backward) => index - 1,
    };
    hits.get(next).map(|row| row.path.clone())
}

fn keeps(
    row: &FileRow,
    hits: &[String],
) -> bool {
    let inside = format!("{}/", row.path);
    hits.iter().any(|hit| {
        hit == &row.path || (matches!(row.kind, FileKind::Directory(_)) && hit.starts_with(&inside))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::expansion::Expansion;
    use crate::git_status::GitStatus;

    fn row(
        path: &str,
        kind: FileKind,
    ) -> FileRow {
        FileRow {
            path: path.into(),
            name: path.rsplit('/').next().unwrap_or(path).into(),
            depth: 0,
            kind,
            status: GitStatus::Clean,
        }
    }

    fn tree() -> Vec<FileRow> {
        let open = FileKind::Directory(Expansion::Expanded);
        vec![
            row("src", open),
            row("src/main.rs", FileKind::File),
            row("src/lib.rs", FileKind::File),
            row("docs", open),
            row("docs/Guide.md", FileKind::File),
            row("README.md", FileKind::File),
        ]
    }

    fn paths(rows: Vec<FileRow>) -> Vec<String> {
        rows.into_iter().map(|row| row.path).collect()
    }

    #[test]
    fn the_highlight_is_the_first_case_insensitive_hit_in_the_name() {
        let cases = [
            ("Guide.md", "gui", Some(0..3)),
            ("main.rs", ".RS", Some(4..7)),
            ("main.rs", "  ", None),
            ("main.rs", "zzz", None),
        ];
        for (name, query, expected) in cases {
            assert_eq!(highlight(name, query), expected, "{name} {query}");
        }
    }

    #[test]
    fn stepping_walks_the_matches_and_wraps_in_both_directions() {
        const F: MatchDirection = MatchDirection::Forward;
        const B: MatchDirection = MatchDirection::Backward;
        let rows = tree();
        let hit = |selected, direction| step_match(&rows, (".rs", selected), direction);
        assert_eq!(hit(None, F).as_deref(), Some("src/main.rs"));
        assert_eq!(hit(None, B).as_deref(), Some("src/lib.rs"));
        assert_eq!(hit(Some("src/main.rs"), F).as_deref(), Some("src/lib.rs"));
        assert_eq!(hit(Some("src/lib.rs"), F).as_deref(), Some("src/main.rs"));
        assert_eq!(hit(Some("src/main.rs"), B).as_deref(), Some("src/lib.rs"));
        assert_eq!(step_match(&rows, ("zzz", None), F), None);
    }

    #[test]
    fn a_query_keeps_matching_names_and_the_folders_that_hold_them() {
        type Case = (&'static str, &'static [&'static str]);
        const CASES: [Case; 6] = [
            (
                "",
                &[
                    "src",
                    "src/main.rs",
                    "src/lib.rs",
                    "docs",
                    "docs/Guide.md",
                    "README.md",
                ],
            ),
            (
                "   ",
                &[
                    "src",
                    "src/main.rs",
                    "src/lib.rs",
                    "docs",
                    "docs/Guide.md",
                    "README.md",
                ],
            ),
            ("main", &["src", "src/main.rs"]),
            ("GUIDE", &["docs", "docs/Guide.md"]),
            (".md", &["docs", "docs/Guide.md", "README.md"]),
            ("zzz", &[]),
        ];
        for (query, expected) in CASES {
            assert_eq!(paths(matching(tree(), query)), expected, "query {query:?}");
        }
    }

    #[test]
    fn a_folder_whose_name_matches_is_kept_without_its_children() {
        assert_eq!(paths(matching(tree(), "docs")), ["docs"]);
    }

    #[test]
    fn slash_and_the_secondary_f_open_the_box() {
        let plain = Modifiers::none();
        let secondary = Modifiers::secondary_key();
        let shift = Modifiers::shift();
        let cases = [
            ("/", plain, true),
            ("/", shift, false),
            ("/", secondary, false),
            ("f", secondary, true),
            ("f", plain, false),
            ("a", plain, false),
        ];
        for (key, modifiers, opens) in cases {
            assert_eq!(opens_search(key, modifiers), opens, "{key} {modifiers:?}");
        }
    }
}
