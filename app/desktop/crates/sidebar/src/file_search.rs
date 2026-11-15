//! The filter box of the Files panel and the rows it keeps.

use gpui::{Entity, Modifiers, Subscription};
use ui::TextInput;

use crate::file_kind::FileKind;
use crate::file_row::FileRow;

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

pub(crate) fn matching(
    rows: Vec<FileRow>,
    query: &str,
) -> Vec<FileRow> {
    let needle = query.trim().to_lowercase();
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
