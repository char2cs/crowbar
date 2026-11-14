use super::*;

fn scratch(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("cb-doc-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    dir.join("hosts.json")
}

#[test]
fn a_saved_list_reads_back() {
    let path = scratch("roundtrip");
    let mut hosts = BTreeMap::new();
    hosts.insert(
        HostId::new("host-1").unwrap(),
        Endpoint::parse("a.example:3737").unwrap(),
    );
    HostsDocument::save(&path, &hosts).unwrap();
    let read = HostsDocument::parse(&path, &std::fs::read(&path).unwrap()).unwrap();
    assert_eq!(read, hosts);
}

#[test]
fn a_corrupt_newer_or_unregistrable_file_is_an_error() {
    let path = scratch("bad");
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
            "local daemon",
        ),
    ] {
        let err = HostsDocument::parse(&path, text.as_bytes()).unwrap_err();
        assert!(err.to_string().contains(wanted), "{text}: {err}");
    }
}
