//! Finding the `localStorage` database the previous desktop app left behind.

use std::path::{Path, PathBuf};

use crate::file_store::FileStore;

const APP_ID: &str = "software.rabbyte.crowbar";
const LINUX_DIR: &str = "localstorage";
const LINUX_EXTENSION: &str = "localstorage";
const MACOS_FILE: &str = "localstorage.sqlite3";

pub(crate) fn under(
    files: &dyn FileStore,
    user_home: &Path,
) -> Option<PathBuf> {
    if cfg!(target_os = "macos") {
        return macos(files, &user_home.join("Library"));
    }
    linux(files, &user_home.join(".local/share"))
}

fn linux(
    files: &dyn FileStore,
    data_home: &Path,
) -> Option<PathBuf> {
    files
        .children(&data_home.join(APP_ID).join(LINUX_DIR))
        .into_iter()
        .find(|path| {
            path.extension().is_some_and(|ext| ext == LINUX_EXTENSION)
                && path
                    .file_name()
                    .is_some_and(|name| name.to_string_lossy().contains("tauri"))
        })
}

fn macos(
    files: &dyn FileStore,
    library: &Path,
) -> Option<PathBuf> {
    let site = library
        .join("WebKit")
        .join(APP_ID)
        .join("WebsiteData")
        .join("Default");
    files
        .children(&site)
        .into_iter()
        .flat_map(|first| files.children(&first))
        .flat_map(|second| files.children(&second.join("LocalStorage")))
        .find(|path| path.file_name().is_some_and(|name| name == MACOS_FILE))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    fn laid_out() -> FakeFiles {
        let files = FakeFiles::default();
        let linux_dir = format!("/share/{APP_ID}/localstorage");
        files.put(&format!("{linux_dir}/tauri_localhost_0.localstorage"), b"");
        files.put(&format!("{linux_dir}/other_0.localstorage"), b"");
        let mac_dir = format!("/lib/WebKit/{APP_ID}/WebsiteData/Default/a/b/LocalStorage");
        files.put(&format!("{mac_dir}/{MACOS_FILE}"), b"");
        files
    }

    #[test]
    fn the_old_database_is_found_on_each_system() {
        let files = laid_out();
        let linux_file = format!("/share/{APP_ID}/localstorage/tauri_localhost_0.localstorage");
        let mac_file =
            format!("/lib/WebKit/{APP_ID}/WebsiteData/Default/a/b/LocalStorage/{MACOS_FILE}");
        assert_eq!(
            linux(&files, Path::new("/share")),
            Some(PathBuf::from(linux_file))
        );
        assert_eq!(
            macos(&files, Path::new("/lib")),
            Some(PathBuf::from(mac_file))
        );
        assert_eq!(linux(&files, Path::new("/none")), None);
    }
}
