//! Finding the `localStorage` database the previous desktop app left behind.

use std::path::{Path, PathBuf};

const APP_ID: &str = "software.rabbyte.crowbar";
const LINUX_DIR: &str = "localstorage";
const LINUX_EXTENSION: &str = "localstorage";
const MACOS_FILE: &str = "localstorage.sqlite3";

pub(crate) fn under(user_home: &Path) -> Option<PathBuf> {
    if cfg!(target_os = "macos") {
        return macos(&user_home.join("Library"));
    }
    linux(&user_home.join(".local/share"))
}

fn children(dir: &Path) -> Vec<PathBuf> {
    let Ok(read) = std::fs::read_dir(dir) else {
        return Vec::new();
    };
    let mut paths: Vec<PathBuf> = read.filter_map(|entry| Some(entry.ok()?.path())).collect();
    paths.sort();
    paths
}

fn linux(data_home: &Path) -> Option<PathBuf> {
    children(&data_home.join(APP_ID).join(LINUX_DIR))
        .into_iter()
        .find(|path| {
            path.extension().is_some_and(|ext| ext == LINUX_EXTENSION)
                && path
                    .file_name()
                    .is_some_and(|name| name.to_string_lossy().contains("tauri"))
        })
}

fn macos(library: &Path) -> Option<PathBuf> {
    let site = library
        .join("WebKit")
        .join(APP_ID)
        .join("WebsiteData")
        .join("Default");
    children(&site)
        .into_iter()
        .flat_map(|first| children(&first))
        .flat_map(|second| children(&second.join("LocalStorage")))
        .find(|path| path.file_name().is_some_and(|name| name == MACOS_FILE))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn touch(path: &Path) {
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(path, b"").unwrap();
    }

    #[test]
    fn the_old_database_is_found_on_each_system() {
        let dir = tempfile::tempdir().unwrap();
        let linux_file = dir
            .path()
            .join("share")
            .join(APP_ID)
            .join("localstorage")
            .join("tauri_localhost_0.localstorage");
        touch(&linux_file);
        touch(
            &dir.path()
                .join("share")
                .join(APP_ID)
                .join("localstorage")
                .join("other_0.localstorage"),
        );
        let mac_file = dir
            .path()
            .join("lib/WebKit")
            .join(APP_ID)
            .join("WebsiteData/Default/a/b/LocalStorage")
            .join(MACOS_FILE);
        touch(&mac_file);
        assert_eq!(linux(&dir.path().join("share")), Some(linux_file));
        assert_eq!(macos(&dir.path().join("lib")), Some(mac_file));
        assert_eq!(linux(&dir.path().join("none")), None);
    }
}
