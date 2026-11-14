use super::*;

#[test]
fn parse_reads_socket_and_log() {
    let out = br#"{"home":"/Users/a/.crowbar","socket":"/tmp/crowbar-1f.sock","log":"/Users/a/.crowbar/logs/daemon.log"}"#;
    let paths = DaemonPaths::parse(out).unwrap();
    assert_eq!(paths.socket, PathBuf::from("/tmp/crowbar-1f.sock"));
    assert_eq!(
        paths.log,
        PathBuf::from("/Users/a/.crowbar/logs/daemon.log")
    );
}

#[test]
fn parse_rejects_relative_or_missing_fields() {
    for out in [
        r#"{"home":"/h","socket":"crowbar.sock","log":"/l"}"#,
        r#"{"home":"/h","socket":"/s.sock","log":"daemon.log"}"#,
        r#"{"home":"/h","log":"/l"}"#,
        r#"{"home":"/h","socket":"/s.sock"}"#,
        r#"{"home":"/h","socket":"/s.sock","log":""}"#,
        r#"{"home":"/h","socket":7,"log":"/l"}"#,
        "starting daemon...",
        "",
    ] {
        assert!(DaemonPaths::parse(out.as_bytes()).is_err(), "{out}");
    }
}

#[test]
fn a_set_home_is_not_replaced_and_an_empty_one_counts_as_unset() {
    let dev = Some(PathBuf::from("/work/.crowbar"));
    assert_eq!(
        child_home(Some("/mine".into()), dev.clone()),
        Some(PathBuf::from("/mine"))
    );
    assert_eq!(child_home(Some(OsString::new()), dev.clone()), dev);
    assert_eq!(child_home(None, dev.clone()), dev);
    assert_eq!(child_home(None, None), None);
}

#[tokio::test]
async fn a_binary_that_cannot_run_is_a_spawn_error() {
    let err = DaemonPaths::resolve(Path::new("/definitely/not/crowbar-api"), None)
        .await
        .unwrap_err();
    assert!(matches!(err, Error::Spawn { .. }), "{err}");
}
