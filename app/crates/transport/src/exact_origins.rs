use protocol::{PaneOrigin, RelayScope};

use crate::origin_policy::OriginPolicy;

/// The shell's webview origins: one fixed list for core panes, and a template with an
/// `{extension}` placeholder for each extension's own origin. Matching is exact, never a prefix.
#[derive(Debug, Clone)]
pub struct ExactOrigins {
    core: Vec<String>,
    extension_template: String,
}

impl ExactOrigins {
    /// Core panes load from `core`; an extension pane from `extension_template` with its id
    /// substituted, for example `crowbar-ext://{extension}`.
    #[must_use]
    pub fn new(core: Vec<String>, extension_template: impl Into<String>) -> Self {
        Self {
            core,
            extension_template: extension_template.into(),
        }
    }
}

impl OriginPolicy for ExactOrigins {
    fn allows(&self, scope: &RelayScope, origin: &str) -> bool {
        if origin.is_empty() || origin == "null" {
            return false;
        }
        match scope.origin() {
            PaneOrigin::Core => self.core.iter().any(|allowed| allowed == origin),
            PaneOrigin::Extension(id) => {
                self.extension_template.replace("{extension}", id.as_str()) == origin
            },
        }
    }
}

#[cfg(test)]
#[path = "exact_origins_tests.rs"]
mod tests;
