use std::collections::BTreeMap;

use crate::shell::git::changed_file::ChangedFile;

#[derive(Debug, Default)]
pub(super) struct Folder<'a> {
    pub(super) name: String,
    pub(super) path: String,
    folders: BTreeMap<String, Folder<'a>>,
    files: Vec<(String, &'a ChangedFile)>,
}

fn segments(path: &str) -> Vec<&str> {
    path.split(['/', '\\'])
        .map(str::trim)
        .filter(|segment| !segment.is_empty())
        .collect()
}

fn child_path(
    parent: &str,
    name: &str,
) -> String {
    if parent.is_empty() {
        return name.to_owned();
    }
    format!("{parent}/{name}")
}

impl<'a> Folder<'a> {
    pub(super) fn of(files: &'a [ChangedFile]) -> Self {
        let mut root = Self::default();
        for file in files {
            root.insert(file);
        }
        root
    }

    fn insert(
        &mut self,
        file: &'a ChangedFile,
    ) {
        let parts = segments(&file.path);
        let Some((name, folders)) = parts.split_last() else {
            return;
        };
        let mut node = self;
        for folder in folders {
            node = node.child(folder);
        }
        node.files.push(((*name).to_owned(), file));
    }

    fn child(
        &mut self,
        name: &str,
    ) -> &mut Self {
        let path = child_path(&self.path, name);
        self.folders
            .entry(name.to_owned())
            .or_insert_with(|| Folder {
                name: name.to_owned(),
                path,
                ..Folder::default()
            })
    }

    fn single(&self) -> Option<&Self> {
        let mut values = self.folders.values();
        let only = values.next()?;
        values.next().is_none().then_some(only)
    }

    pub(super) fn chain(&self) -> (String, &Self) {
        let mut names = vec![self.name.as_str()];
        let mut node = self;
        while let (true, Some(only)) = (node.files.is_empty(), node.single()) {
            names.push(only.name.as_str());
            node = only;
        }
        (names.join("/"), node)
    }

    pub(super) fn sorted_folders(&self) -> Vec<&Self> {
        let mut folders: Vec<&Self> = self.folders.values().collect();
        folders.sort_by_key(|folder| folder.name.to_lowercase());
        folders
    }

    pub(super) fn sorted_files(&self) -> Vec<&(String, &'a ChangedFile)> {
        let mut files: Vec<_> = self.files.iter().collect();
        files.sort_by_key(|(_, file)| file.path.to_lowercase());
        files
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::listing::GitStatus;

    type Case = (
        &'static [&'static str],
        &'static str,
        &'static [&'static str],
    );

    const CASES: &[Case] = &[
        (&["a/b/c/x.ts"], "/a/b/c", &["x.ts"]),
        (&["a/x.ts", "a/b/y.ts"], "/a", &["x.ts"]),
        (&["a\\b\\y.ts", " a / b /x.ts"], "/a/b", &["x.ts", "y.ts"]),
    ];

    fn file(path: &str) -> ChangedFile {
        ChangedFile {
            path: path.into(),
            status: GitStatus::Modified,
            added: 0,
            deleted: 0,
        }
    }

    #[test]
    fn a_chain_runs_down_lone_folders_that_hold_no_files() {
        for (paths, label, names) in CASES {
            let files: Vec<ChangedFile> = paths.iter().map(|path| file(path)).collect();
            let root = Folder::of(&files);
            let (chain, end) = root.chain();
            let listed: Vec<&str> = end.sorted_files().iter().map(|(n, _)| n.as_str()).collect();
            assert_eq!(
                (chain.as_str(), listed.as_slice()),
                (*label, *names),
                "{paths:?}"
            );
        }
    }
}
