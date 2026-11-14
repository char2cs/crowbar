use std::path::PathBuf;

/// Settings read from the command line and environment.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AppOptions {
    pub(crate) panes_dir: PathBuf,
    pub(crate) data_dir: Option<PathBuf>,
    pub(crate) chat_entry: String,
}

impl AppOptions {
    pub(crate) fn from_env(vars: impl Fn(&str) -> Option<String>) -> Self {
        Self {
            panes_dir: vars("CROWBAR_PANES_DIR").map_or_else(|| PathBuf::from("panes"), PathBuf::from),
            data_dir: vars("CROWBAR_WEBVIEW_DATA").map(PathBuf::from),
            chat_entry: vars("CROWBAR_CHAT_ENTRY").unwrap_or_else(|| "chat/index.html".into()),
        }
    }
}

#[cfg(test)]
#[path = "app_options_tests.rs"]
mod tests;
