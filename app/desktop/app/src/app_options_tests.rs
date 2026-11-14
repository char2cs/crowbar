use super::*;

#[test]
fn defaults_and_overrides() {
    let defaults = AppOptions::from_env(|_| None);
    assert_eq!(defaults.panes_dir, PathBuf::from("panes"), "panes dir");
    assert_eq!((defaults.data_dir, defaults.chat_entry.as_str()), (None, "chat/index.html"), "rest");
    let set = AppOptions::from_env(|key| match key {
        "CROWBAR_PANES_DIR" => Some("/p".into()),
        "CROWBAR_WEBVIEW_DATA" => Some("/d".into()),
        "CROWBAR_CHAT_ENTRY" => Some("index.html".into()),
        _ => None,
    });
    assert_eq!(set.panes_dir, PathBuf::from("/p"), "panes dir");
    assert_eq!(set.data_dir, Some(PathBuf::from("/d")), "data dir");
    assert_eq!(set.chat_entry, "index.html", "entry");
}
