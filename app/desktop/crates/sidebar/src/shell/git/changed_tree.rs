//! The Changes panel's tree: changed files grouped under their folders.

use std::collections::{BTreeMap, BTreeSet};

use crate::shell::Expansion;
use crate::shell::git::changed_file::ChangedFile;
use crate::shell::git::changed_row::ChangedRow;
use crate::shell::git::changes::Changes;

mod folder;

use folder::Folder;

type Marks<'m> = (&'m BTreeSet<String>, &'m BTreeMap<String, u32>);

fn expansion(
    folded: &BTreeSet<String>,
    path: &str,
) -> Expansion {
    if folded.contains(path) {
        return Expansion::Collapsed;
    }
    Expansion::Expanded
}

fn file_row(
    (name, file): (&str, &ChangedFile),
    depth: u8,
    threads: &BTreeMap<String, u32>,
) -> ChangedRow {
    ChangedRow::File {
        file: file.clone(),
        name: name.to_owned(),
        depth,
        threads: threads.get(&file.path).copied().unwrap_or_default(),
    }
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
    let row = ChangedRow::Folder {
        path: node.path.clone(),
        label,
        depth,
        expansion,
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
        level(&Folder::of(&self.files), 0, (folded, &self.threads))
    }
}

#[cfg(test)]
mod tests {
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
            name: "folders come before files, each level sorted",
            paths: &[
                "README.md",
                "src/pricing.ts",
                "src/pricing.test.ts",
                "src/stock/a.ts",
            ],
            folded: &[],
            outline: &[
                "0 src/",
                "1 stock/",
                "2 a.ts",
                "1 pricing.test.ts",
                "1 pricing.ts (2)",
                "0 README.md",
            ],
        },
        Case {
            name: "a chain of lone folders is one row",
            paths: &["chain/only/x.ts", "chain/only/y.ts"],
            folded: &[],
            outline: &["0 chain/only/", "1 x.ts", "1 y.ts"],
        },
        Case {
            name: "folding a chain hides its files",
            paths: &["chain/only/x.ts", "chain/only/y.ts"],
            folded: &["chain/only"],
            outline: &["0 chain/only/ folded"],
        },
        Case {
            name: "a trailing slash names the entry",
            paths: &["utils/"],
            folded: &[],
            outline: &["0 utils"],
        },
    ];

    fn file(path: &str) -> ChangedFile {
        ChangedFile {
            path: path.into(),
            status: GitStatus::Modified,
            added: 1,
            deleted: 0,
        }
    }

    fn changes(paths: &[&str]) -> Changes {
        Changes {
            files: paths.iter().map(|path| file(path)).collect(),
            threads: BTreeMap::from([("src/pricing.ts".to_owned(), 2)]),
            ..Changes::default()
        }
    }

    fn suffix(
        expansion: Expansion,
        threads: u32,
    ) -> String {
        if expansion == Expansion::Collapsed {
            return " folded".into();
        }
        if threads == 0 {
            return String::new();
        }
        format!(" ({threads})")
    }

    fn line(row: &ChangedRow) -> String {
        let (label, depth, end) = match row {
            ChangedRow::Folder {
                label,
                depth,
                expansion,
                ..
            } => (format!("{label}/"), depth, suffix(*expansion, 0)),
            ChangedRow::File {
                name,
                depth,
                threads,
                ..
            } => (name.clone(), depth, suffix(Expansion::Expanded, *threads)),
        };
        format!("{depth} {label}{end}")
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
