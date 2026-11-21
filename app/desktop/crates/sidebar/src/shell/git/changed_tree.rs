//! The Changes panel's tree: changed files grouped under their folders.

use std::collections::BTreeSet;

use crate::shell::Expansion;
use crate::shell::git::changed_file::ChangedFile;
use crate::shell::git::changed_row::ChangedRow;
use crate::shell::git::changes::Changes;

mod folder;

use folder::Folder;

type FileMark = (String, String);

type Marks<'m> = (&'m BTreeSet<String>, &'m Changes);

fn expansion(
    folded: &BTreeSet<String>,
    path: &str,
) -> Expansion {
    if folded.contains(path) {
        return Expansion::Collapsed;
    }
    Expansion::Expanded
}

fn is_viewed(
    changes: &Changes,
    file: &ChangedFile,
) -> bool {
    changes.viewed.get(&file.path) == Some(&file.signature)
}

fn file_row(
    (name, file): (&str, &ChangedFile),
    depth: u8,
    changes: &Changes,
) -> ChangedRow {
    ChangedRow::File {
        file: file.clone(),
        name: name.to_owned(),
        depth,
        threads: changes.threads.get(&file.path).copied().unwrap_or_default(),
        viewed: is_viewed(changes, file),
    }
}

fn marks_under(
    node: &Folder<'_>,
    changes: &Changes,
) -> (Vec<FileMark>, usize) {
    let files = node.all_files();
    let viewed = files.iter().filter(|file| is_viewed(changes, file)).count();
    let marks = files
        .into_iter()
        .map(|file| (file.path.clone(), file.signature.clone()))
        .collect();
    (marks, viewed)
}

fn level(
    node: &Folder<'_>,
    depth: u8,
    marks: Marks<'_>,
) -> Vec<ChangedRow> {
    let folders = node
        .sorted_folders()
        .into_iter()
        .flat_map(|child| folder_rows(child, depth, marks));
    let files = node
        .sorted_files()
        .into_iter()
        .map(|(name, file)| file_row((name, file), depth, marks.1));
    folders.chain(files).collect()
}

fn folder_rows(
    start: &Folder<'_>,
    depth: u8,
    marks: Marks<'_>,
) -> Vec<ChangedRow> {
    let (label, node) = start.chain();
    let expansion = expansion(marks.0, &node.path);
    let (files, viewed) = marks_under(node, marks.1);
    let row = ChangedRow::Folder {
        path: node.path.clone(),
        label,
        depth,
        expansion,
        files,
        viewed,
    };
    let inside = match expansion {
        Expansion::Expanded => level(node, depth.saturating_add(1), marks),
        Expansion::Collapsed => Vec::new(),
    };
    std::iter::once(row).chain(inside).collect()
}

impl Changes {
    /// The tree's lines: at each level the folders by name, each followed by its contents unless
    /// it is in `folded`, then that level's files by path.
    #[must_use]
    pub fn rows(
        &self,
        folded: &BTreeSet<String>,
    ) -> Vec<ChangedRow> {
        level(&Folder::of(&self.files), 0, (folded, self))
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;
    use crate::shell::listing::GitStatus;

    struct Case {
        name: &'static str,
        paths: &'static [&'static str],
        folded: &'static [&'static str],
        outline: &'static [&'static str],
    }

    const CASES: &[Case] = &[
        Case {
            name: "folders come first; a folder counts the viewed files under it",
            paths: &[
                "README.md",
                "src/pricing.ts",
                "src/pricing.test.ts",
                "src/stock/a.ts",
            ],
            folded: &[],
            outline: &[
                "0 src/ 1/3",
                "1 stock/ 0/1",
                "2 a.ts",
                "1 pricing.test.ts",
                "1 pricing.ts (2) ✓",
                "0 README.md",
            ],
        },
        Case {
            name: "a chain of lone folders is one row",
            paths: &["chain/only/x.ts", "chain/only/y.ts"],
            folded: &[],
            outline: &["0 chain/only/ 0/2", "1 x.ts", "1 y.ts"],
        },
        Case {
            name: "folding a chain hides its files",
            paths: &["chain/only/x.ts", "chain/only/y.ts"],
            folded: &["chain/only"],
            outline: &["0 chain/only/ 0/2 folded"],
        },
        Case {
            name: "an untracked folder is named by its whole path, as git reports it",
            paths: &["src/utils/", "src/pricing.ts"],
            folded: &[],
            outline: &["0 src/ 1/2", "1 pricing.ts (2) ✓", "1 src/utils/"],
        },
    ];

    fn file(path: &str) -> ChangedFile {
        ChangedFile {
            path: path.into(),
            status: GitStatus::Modified,
            added: 1,
            deleted: 0,
            signature: "s".into(),
        }
    }

    fn changes(paths: &[&str]) -> Changes {
        Changes {
            files: paths.iter().map(|path| file(path)).collect(),
            threads: BTreeMap::from([("src/pricing.ts".to_owned(), 2)]),
            viewed: BTreeMap::from([
                ("src/pricing.ts".to_owned(), "s".to_owned()),
                ("src/stock/a.ts".to_owned(), "changed since".to_owned()),
            ]),
            ..Changes::default()
        }
    }

    fn folder_line(row: &ChangedRow) -> Option<String> {
        let ChangedRow::Folder {
            label,
            depth,
            expansion,
            files,
            viewed,
            ..
        } = row
        else {
            return None;
        };
        let folded = (*expansion == Expansion::Collapsed).then_some(" folded");
        let folded = folded.unwrap_or_default();
        Some(format!("{depth} {label}/ {viewed}/{}{folded}", files.len()))
    }

    fn file_line(row: &ChangedRow) -> Option<String> {
        let ChangedRow::File {
            name,
            depth,
            threads,
            viewed,
            ..
        } = row
        else {
            return None;
        };
        let threads = (*threads > 0).then(|| format!(" ({threads})"));
        let threads = threads.unwrap_or_default();
        let tick = viewed.then_some(" ✓").unwrap_or_default();
        Some(format!("{depth} {name}{threads}{tick}"))
    }

    fn line(row: &ChangedRow) -> String {
        folder_line(row)
            .or_else(|| file_line(row))
            .unwrap_or_default()
    }

    #[test]
    fn changed_files_sit_under_their_folders() {
        for case in CASES {
            let folded = case.folded.iter().map(|path| (*path).to_owned()).collect();
            let rows = changes(case.paths).rows(&folded);
            let outline: Vec<String> = rows.iter().map(line).collect();
            assert_eq!(outline, case.outline, "{}", case.name);
        }
    }
}
