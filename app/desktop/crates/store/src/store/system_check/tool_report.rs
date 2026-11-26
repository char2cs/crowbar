use serde::Deserialize;

/// What the daemon found out about one tool.
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct ToolReport {
    /// Whether the tool runs.
    #[serde(default)]
    pub installed: bool,
    /// Whether the tool is signed in; only the forge CLIs report it.
    #[serde(default)]
    pub authed: bool,
    /// The version line, when the tool has one.
    #[serde(default)]
    pub version: Option<String>,
}
