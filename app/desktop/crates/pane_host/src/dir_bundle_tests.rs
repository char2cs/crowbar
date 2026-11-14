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

#[test]
fn paths_cannot_leave_the_directory() {
    let root = fixture("escape");
    let secret = format!("../secret-{}-escape", std::process::id());
    let bundle = DirBundle::new(root);
    assert!(bundle.read(&secret).is_none(), "parent traversal");
    assert!(bundle.read("/etc/passwd").is_none(), "absolute path");
    assert!(bundle.read("assets/../../x").is_none(), "nested traversal");
    assert!(bundle.read("missing.js").is_none(), "missing file");
}
