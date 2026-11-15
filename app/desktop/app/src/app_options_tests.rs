use super::*;

#[test]
fn defaults_follow_the_home_directory() {
    let options = AppOptions::from_env(|key| (key == "HOME").then(|| "/home/u".into()));
    assert_eq!(options.home, PathBuf::from("/home/u/.crowbar"), "home");
    assert_eq!(
        options.state_dir(),
        PathBuf::from("/home/u/.crowbar/shell"),
        "state"
    );
    assert_eq!(
        (
            options.panes_dir,
            options.chat_entry.as_str(),
            options.daemon_binary
        ),
        (PathBuf::from("panes"), "chat/index.html", None),
        "rest"
    );
}

#[test]
fn the_environment_overrides_everything() {
    let options = AppOptions::from_env(|key| match key {
        "CROWBAR_HOME" => Some("/h".into()),
        "CROWBAR_PANES_DIR" => Some("/p".into()),
        "CROWBAR_CHAT_ENTRY" => Some("index.html".into()),
        "CROWBAR_API_BIN" => Some("/bin/api".into()),
        _ => None,
    });
    assert_eq!(options.home, PathBuf::from("/h"), "home");
    assert_eq!(options.panes_dir, PathBuf::from("/p"), "panes");
    assert_eq!(options.chat_entry, "index.html", "entry");
    assert_eq!(
        options.daemon_binary,
        Some(PathBuf::from("/bin/api")),
        "binary"
    );
}

#[test]
fn an_empty_home_variable_counts_as_unset() {
    let options = AppOptions::from_env(|key| match key {
        "CROWBAR_HOME" => Some(String::new()),
        "HOME" => Some("/home/u".into()),
        _ => None,
    });
    assert_eq!(options.home, PathBuf::from("/home/u/.crowbar"), "fallback");
}
