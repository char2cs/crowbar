use std::sync::Arc;

use super::*;

struct One;

impl BundleSource for One {
    fn read(&self, path: &str) -> Option<BundleAsset> {
        (path == "index.html").then(|| BundleAsset {
            mime: "text/html",
            bytes: b"<p>".to_vec(),
        })
    }
}

#[test]
fn sources_are_shared_as_trait_objects() {
    let source: Arc<dyn BundleSource> = Arc::new(One);
    assert!(source.read("index.html").is_some(), "present");
    assert!(source.read("missing.js").is_none(), "absent");
}
