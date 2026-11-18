//! The visible rows of the tree: each folder's own children, and below every opened folder its
//! rows or the note that stands in for them.

mod folder_note;
mod folder_row;
mod row_element;
mod shown;

pub(crate) use folder_row::FolderRow;
pub(crate) use row_element::{Toggle, tree_row};
pub(crate) use shown::Shown;

use store::{BrowseEntry, EntryKind, FolderListing};
use ui::Pose;

use crate::tree::folder_note::FolderNote;

const FILE_CAP: usize = 500;

pub(crate) fn children_of<'a>(
    dir: &str,
    entries: &'a [BrowseEntry],
) -> Vec<&'a BrowseEntry> {
    let prefix = with_slash(dir);
    entries
        .iter()
        .filter(|entry| {
            entry.path.len() > prefix.len()
                && entry.path.starts_with(&prefix)
                && entry
                    .path
                    .get(prefix.len()..)
                    .is_some_and(|rest| !rest.contains('/'))
        })
        .collect()
}

fn with_slash(dir: &str) -> String {
    if dir.ends_with('/') {
        return dir.to_owned();
    }
    format!("{dir}/")
}

fn note_for(
    dir: &str,
    listing: Option<&FolderListing>,
) -> Option<FolderNote> {
    match listing {
        None => Some(FolderNote::Waiting),
        Some(FolderListing::Loading) => Some(FolderNote::Loading),
        Some(FolderListing::Failed(reason)) => Some(FolderNote::Failed(reason.clone())),
        Some(FolderListing::Loaded(entries)) if children_of(dir, entries).is_empty() => {
            Some(FolderNote::Empty)
        },
        Some(FolderListing::Loaded(_)) => None,
    }
}

fn nested(
    entry: &BrowseEntry,
    shown: &Shown<'_>,
    level: usize,
) -> Vec<FolderRow> {
    let listing = shown.source.listing(&entry.path);
    if let Some(note) = note_for(&entry.path, listing.as_ref()) {
        return vec![FolderRow::Note { level, note }];
    }
    match listing {
        Some(FolderListing::Loaded(children)) => rows(&entry.path, &children, shown, level),
        _ => Vec::new(),
    }
}

fn entry_rows(
    entry: &BrowseEntry,
    shown: &Shown<'_>,
    level: usize,
) -> Vec<FolderRow> {
    let open = Pose::from(shown.expanded.contains(&entry.path));
    let row = FolderRow::Entry {
        entry: entry.clone(),
        level,
        open,
    };
    if entry.kind != EntryKind::Folder || open == Pose::Off {
        return vec![row];
    }
    let mut rows = vec![row];
    rows.extend(nested(entry, shown, level + 1));
    rows
}

pub(crate) fn rows(
    parent: &str,
    entries: &[BrowseEntry],
    shown: &Shown<'_>,
    level: usize,
) -> Vec<FolderRow> {
    let own = children_of(parent, entries);
    let (folders, files): (Vec<_>, Vec<_>) = own
        .into_iter()
        .partition(|entry| entry.kind == EntryKind::Folder);
    let visible = folders.iter().chain(files.iter().take(FILE_CAP));
    let cut = (files.len() > FILE_CAP).then(|| FolderRow::Note {
        level,
        note: FolderNote::MoreFiles(files.len() - FILE_CAP),
    });
    visible
        .flat_map(|entry| entry_rows(entry, shown, level))
        .chain(cut)
        .collect()
}

#[cfg(test)]
mod tests {
    use std::collections::{BTreeSet, HashMap};

    use super::*;
    use crate::folder_source::FolderReader;
    use crate::folder_source::FolderRequests;

    struct Listed(HashMap<String, FolderListing>);

    impl FolderReader for Listed {
        fn host_name(&self) -> String {
            "Box".to_owned()
        }

        fn can_ask(&self) -> bool {
            true
        }

        fn listing(
            &self,
            path: &str,
        ) -> Option<FolderListing> {
            self.0.get(path).cloned()
        }
    }

    impl FolderRequests for Listed {
        fn request(
            &self,
            _: &str,
        ) {
        }

        fn retry(
            &self,
            _: &str,
        ) {
        }
    }

    fn entry(
        path: &str,
        kind: EntryKind,
    ) -> BrowseEntry {
        let name = path.rsplit('/').next().unwrap_or_default();
        BrowseEntry {
            name: name.to_owned(),
            path: path.to_owned(),
            kind,
        }
    }

    fn loaded(entries: &[(&str, EntryKind)]) -> FolderListing {
        let list = entries
            .iter()
            .map(|(path, kind)| entry(path, *kind))
            .collect();
        FolderListing::Loaded(list)
    }

    fn mark(open: Pose) -> &'static str {
        if open == Pose::On {
            return "+";
        }
        ""
    }

    fn summary(rows: &[FolderRow]) -> Vec<String> {
        rows.iter()
            .map(|row| match row {
                FolderRow::Entry { entry, level, open } => {
                    format!("{}@{level}{}", entry.name, mark(*open))
                },
                FolderRow::Note { level, note } => format!("{}@{level}", note.text("Box")),
            })
            .collect()
    }

    fn listings() -> HashMap<String, FolderListing> {
        let root = loaded(&[
            ("/b.txt", EntryKind::File),
            ("/srv", EntryKind::Folder),
            ("/usr", EntryKind::Folder),
            ("/srv/stray/deep", EntryKind::Folder),
        ]);
        let srv = loaded(&[
            ("/srv/app", EntryKind::Folder),
            ("/srv/a.rs", EntryKind::File),
        ]);
        HashMap::from([
            ("/".to_owned(), root),
            ("/srv".to_owned(), srv),
            ("/usr".to_owned(), FolderListing::Loading),
        ])
    }

    type Case = (
        &'static str,
        &'static [&'static str],
        &'static [&'static str],
    );

    const CASES: [Case; 5] = [
        (
            "folders come before files and nothing deeper shows",
            &[],
            &["srv@1", "usr@1", "b.txt@1"],
        ),
        (
            "an opened folder shows its own children under it",
            &["/srv"],
            &["srv@1+", "app@2", "a.rs@2", "usr@1", "b.txt@1"],
        ),
        (
            "an opened folder still loading shows a note",
            &["/usr"],
            &["srv@1", "usr@1+", "Loading…@2", "b.txt@1"],
        ),
        (
            "a folder opened under a closed one stays hidden",
            &["/srv/app"],
            &["srv@1", "usr@1", "b.txt@1"],
        ),
        (
            "a path that is no row opens nothing",
            &["/gone"],
            &["srv@1", "usr@1", "b.txt@1"],
        ),
    ];

    #[test]
    fn the_rows_follow_the_listings_and_the_folders_the_user_opened() {
        let source = Listed(listings());
        let root = loaded_root(&source);
        for (name, opened, expected) in CASES {
            assert_eq!(rows_when(&source, &root, opened), expected, "{name}");
        }
    }

    fn rows_when(
        source: &Listed,
        root: &[BrowseEntry],
        opened: &[&str],
    ) -> Vec<String> {
        let expanded: BTreeSet<String> = opened.iter().map(|p| (*p).to_owned()).collect();
        let shown = Shown {
            source,
            expanded: &expanded,
        };
        summary(&rows("/", root, &shown, 1))
    }

    fn loaded_root(source: &Listed) -> Vec<BrowseEntry> {
        match source.0["/"].clone() {
            FolderListing::Loaded(root) => root,
            other => panic!("the root is loaded, got {other:?}"),
        }
    }

    #[test]
    fn an_unasked_loading_failed_or_empty_folder_each_has_its_note() {
        let empty = loaded(&[]);
        let failed = FolderListing::Failed("denied".into());
        assert_eq!(note_for("/a", None), Some(FolderNote::Waiting));
        assert_eq!(
            note_for("/a", Some(&FolderListing::Loading)),
            Some(FolderNote::Loading)
        );
        assert_eq!(
            note_for("/a", Some(&failed)),
            Some(FolderNote::Failed("denied".into()))
        );
        assert_eq!(note_for("/a", Some(&empty)), Some(FolderNote::Empty));
        assert_eq!(
            note_for("/a", Some(&loaded(&[("/a/b", EntryKind::File)]))),
            None
        );
    }

    #[test]
    fn files_past_the_cap_are_cut_with_a_count() {
        let many: Vec<(String, EntryKind)> = (0..503)
            .map(|index| (format!("/f{index:03}"), EntryKind::File))
            .collect();
        let pairs: Vec<(&str, EntryKind)> = many.iter().map(|(p, k)| (p.as_str(), *k)).collect();
        let FolderListing::Loaded(entries) = loaded(&pairs) else {
            panic!("loaded");
        };
        let source = Listed(HashMap::new());
        let expanded = BTreeSet::new();
        let shown = Shown {
            source: &source,
            expanded: &expanded,
        };
        let all = rows("/", &entries, &shown, 1);
        assert_eq!(all.len(), 501);
        assert_eq!(
            summary(&all).last().map(String::as_str),
            Some("3 more files not shown@1")
        );
    }
}
