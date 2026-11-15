use std::path::{Component, Path, PathBuf};

use crate::bundle_asset::BundleAsset;
use crate::bundle_source::BundleSource;
use crate::mime::mime_for;

/// Serves a bundle from a directory. Paths that would leave the directory are not found.
#[derive(Debug, Clone)]
pub struct DirBundle {
    root: PathBuf,
}

fn index_for(path: &str) -> String {
    if path.is_empty() || path.ends_with('/') {
        return format!("{path}index.html");
    }
    path.to_owned()
}

impl DirBundle {
    /// A bundle rooted at `root`.
    #[must_use]
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    fn resolve(
        &self,
        path: &str,
    ) -> Option<PathBuf> {
        let relative = index_for(path);
        let confined = Path::new(&relative)
            .components()
            .all(|component| matches!(component, Component::Normal(_)));
        confined.then(|| self.root.join(relative))
    }
}

impl BundleSource for DirBundle {
    fn read(
        &self,
        path: &str,
    ) -> Option<BundleAsset> {
        let file = self.resolve(path)?;
        let bytes = std::fs::read(&file).ok()?;
        Some(BundleAsset {
            mime: mime_for(&file.to_string_lossy()),
            bytes,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("pane_host-{}-{name}", std::process::id()));
        std::fs::create_dir_all(dir.join("assets")).unwrap();
        std::fs::write(dir.join("index.html"), "<p>root</p>").unwrap();
        std::fs::write(dir.join("assets/app.js"), "1").unwrap();
        std::fs::write(
            dir.parent()
                .unwrap()
                .join(format!("secret-{}-{name}", std::process::id())),
            "no",
        )
        .unwrap();
        dir
    }

    #[test]
    fn serves_files_with_their_media_type() {
        let bundle = DirBundle::new(fixture("serve"));
        let js = bundle.read("assets/app.js").unwrap();
        assert_eq!(js.mime, "text/javascript; charset=utf-8", "mime");
        assert_eq!(js.bytes, b"1", "bytes");
    }

    #[test]
    fn empty_and_directory_paths_name_index_html() {
        let bundle = DirBundle::new(fixture("index"));
        assert_eq!(bundle.read("").unwrap().bytes, b"<p>root</p>", "root");
        assert_eq!(
            bundle.read("assets/").map(|a| a.bytes),
            None,
            "directory without index"
        );
    }

    const CASES: &[(&str, &str)] = &[
        ("parent traversal", "../secret-ID-escape"),
        ("absolute path", "/etc/passwd"),
        ("nested traversal", "assets/../../x"),
        ("missing file", "missing.js"),
    ];

    #[test]
    fn paths_cannot_leave_the_directory() {
        let bundle = DirBundle::new(fixture("escape"));
        let served: Vec<&str> = CASES
            .iter()
            .filter(|(_, path)| {
                let path = path.replace("ID", &std::process::id().to_string());
                bundle.read(&path).is_some()
            })
            .map(|(name, _)| *name)
            .collect();
        assert!(served.is_empty(), "served although refused: {served:?}");
    }
}
