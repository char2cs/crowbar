use std::path::{Component, Path, PathBuf};

use crate::catalog::pages::bundle_asset::BundleAsset;
use crate::catalog::pages::bundle_source::BundleSource;
use crate::catalog::pages::mime::mime_for;

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

    const INDEX: &[u8] = b"<p>root</p>";
    type Case = (&'static str, &'static str, Option<&'static [u8]>);
    const CASES: &[Case] = &[
        ("a file", "assets/app.js", Some(b"1")),
        ("the empty path names the index", "", Some(INDEX)),
        ("a directory without an index", "assets/", None),
        ("parent traversal", "../secret", None),
        ("absolute path", "/etc/passwd", None),
        ("nested traversal", "assets/../../x", None),
        ("missing file", "missing.js", None),
    ];

    #[ignore = "real-infra: serves a real directory and refuses paths that leave it, which is about real path resolution"]
    fn fixture() -> PathBuf {
        let dir = std::env::temp_dir().join(format!("pane_host-{}", std::process::id()));
        std::fs::create_dir_all(dir.join("assets")).unwrap();
        std::fs::write(dir.join("index.html"), INDEX).unwrap();
        std::fs::write(dir.join("assets/app.js"), "1").unwrap();
        std::fs::write(dir.parent().unwrap().join("secret"), "no").unwrap();
        dir
    }

    #[test]
    #[ignore = "real-infra: serves a real directory and refuses paths that leave it, which is about real path resolution"]
    fn serves_files_confined_to_the_directory_with_their_media_type() {
        let bundle = DirBundle::new(fixture());
        for (name, path, expected) in CASES {
            let served = bundle.read(path).map(|asset| asset.bytes);
            assert_eq!(served.as_deref(), *expected, "{name}");
        }
        let js = bundle.read("assets/app.js").unwrap();
        assert_eq!(js.mime, "text/javascript; charset=utf-8", "mime");
    }
}
