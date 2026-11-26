//! Finding the `IndexedDB` databases where develop and the shell's own editor pages kept drafts.

use std::path::{Path, PathBuf};

use crate::live::Files;

const APP_ID: &str = "software.rabbyte.crowbar";
const DATABASES: [&str; 3] = ["databases", "indexeddb", "v1"];
const DATABASE_FILE: &str = "IndexedDB.sqlite3";

pub(crate) fn databases(
    files: &dyn Files,
    (user_home, webview): (&Path, &Path),
) -> Vec<PathBuf> {
    let develop = user_home.join(".local/share").join(APP_ID);
    let pages = files.children(webview);
    std::iter::once(develop)
        .chain(pages)
        .flat_map(|root| under(files, &root))
        .collect()
}

fn under(
    files: &dyn Files,
    root: &Path,
) -> Vec<PathBuf> {
    let versions = DATABASES
        .iter()
        .fold(root.to_path_buf(), |path, part| path.join(part));
    files
        .children(&versions)
        .into_iter()
        .flat_map(|origin| files.children(&origin))
        .flat_map(|database| files.children(&database))
        .filter(|path| path.file_name().is_some_and(|name| name == DATABASE_FILE))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    #[test]
    fn develop_s_and_each_page_profile_s_databases_are_found() {
        let files = FakeFiles::default();
        let found = [
            "/home/u/.local/share/software.rabbyte.crowbar/databases/indexeddb/v1/tauri_localhost_0/AB/IndexedDB.sqlite3",
            "/state/webview/core/databases/indexeddb/v1/crowbar_local_0/AB/IndexedDB.sqlite3",
        ];
        let other =
            "/state/webview/core/databases/indexeddb/v1/crowbar_local_0/AB/IndexedDB.sqlite3-wal";
        for path in found.iter().chain([&other]) {
            files.write(Path::new(path), b"").unwrap();
        }
        let databases = databases(&files, (Path::new("/home/u"), Path::new("/state/webview")));
        let expected: Vec<PathBuf> = found.iter().map(PathBuf::from).collect();
        assert_eq!(databases, expected);
    }
}
