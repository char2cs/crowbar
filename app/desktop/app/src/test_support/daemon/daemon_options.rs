use std::path::Path;

use crate::app_options::AppOptions;

pub(crate) fn options_without_a_daemon() -> AppOptions {
    AppOptions {
        panes_dir: None,
        chat_entry: "chat/index.html".into(),
        home: Path::new("/nonexistent-crowbar-home").to_path_buf(),
        daemon_binary: Some(Path::new("/nonexistent-crowbar-home/daemon").to_path_buf()),
    }
}
