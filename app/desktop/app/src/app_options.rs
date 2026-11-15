use std::path::PathBuf;

/// Settings read from the environment.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AppOptions {
    pub(crate) panes_dir: PathBuf,
    pub(crate) chat_entry: String,
    pub(crate) home: PathBuf,
    pub(crate) daemon_binary: Option<PathBuf>,
}

impl AppOptions {
    pub(crate) fn from_env(vars: impl Fn(&str) -> Option<String>) -> Self {
        let home = vars("CROWBAR_HOME")
            .filter(|home| !home.is_empty())
            .or_else(|| vars("HOME").map(|home| format!("{home}/.crowbar")))
            .map_or_else(|| PathBuf::from(".crowbar"), PathBuf::from);
        Self {
            panes_dir: vars("CROWBAR_PANES_DIR")
                .map_or_else(|| PathBuf::from("panes"), PathBuf::from),
            chat_entry: vars("CROWBAR_CHAT_ENTRY").unwrap_or_else(|| "chat/index.html".into()),
            home,
            daemon_binary: vars("CROWBAR_API_BIN").map(PathBuf::from),
        }
    }

    /// Where the shell keeps its own files: layout, sidebar preferences, web view data.
    pub(crate) fn state_dir(&self) -> PathBuf {
        self.home.join("shell")
    }
}

#[cfg(test)]
#[path = "app_options_tests.rs"]
mod tests;
