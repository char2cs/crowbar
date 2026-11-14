use std::os::unix::fs::PermissionsExt;

use tempfile::TempDir;

use super::*;

fn scratch() -> (TempDir, PathBuf) {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("sub").join("hosts.json");
    (dir, path)
}

fn one_host() -> BTreeMap<HostId, Endpoint> {
    let mut hosts = BTreeMap::new();
    hosts.insert(
        HostId::new("host-1").unwrap(),
        Endpoint::parse("a.example:3737").unwrap(),
    );
    hosts
}

#[test]
fn a_saved_list_reads_back_privately_and_leaves_no_temp_file() {
    let (_dir, path) = scratch();
    HostsDocument::save(&path, &one_host()).unwrap();
    let read = HostsDocument::parse(&path, &std::fs::read(&path).unwrap()).unwrap();
    assert_eq!(read, one_host());
    let mode = |p: &Path| std::fs::metadata(p).unwrap().permissions().mode() & 0o777;
    assert_eq!(mode(&path), PRIVATE_FILE, "file");
    assert_eq!(mode(path.parent().unwrap()), PRIVATE_DIR, "directory");
    assert!(!staged_name(&path).exists(), "temp copy renamed away");
}

#[test]
fn a_failed_save_removes_its_temp_copy() {
    let (_dir, path) = scratch();
    HostsDocument::save(&path, &one_host()).unwrap();
    std::fs::remove_file(&path).unwrap();
    std::fs::create_dir(&path).unwrap();
    assert!(
        HostsDocument::save(&path, &one_host()).is_err(),
        "cannot replace a directory"
    );
    assert!(!staged_name(&path).exists(), "no stray temp copy");
}

#[test]
fn a_corrupt_newer_or_unregistrable_file_is_an_error() {
    let path = PathBuf::from("hosts.json");
    for (text, wanted) in [
        ("{not json", "corrupt"),
        (r#"{"version":9,"hosts":[]}"#, "version 9"),
        (
            r#"{"version":1,"hosts":[{"id":"Bad Id","endpoint":"a:1"}]}"#,
            "corrupt",
        ),
        (
            r#"{"version":1,"hosts":[{"id":"ok","endpoint":"nope"}]}"#,
            "corrupt",
        ),
        (
            r#"{"version":1,"hosts":[{"id":"localhost","endpoint":"a:1"}]}"#,
            "host localhost",
        ),
        (
            r#"{"version":1,"hosts":[{"id":"a","endpoint":"a:1"},{"id":"a","endpoint":"b:2"}]}"#,
            "host a is listed twice",
        ),
    ] {
        let err = HostsDocument::parse(&path, text.as_bytes()).unwrap_err();
        assert!(err.to_string().contains(wanted), "{text}: {err}");
    }
}
