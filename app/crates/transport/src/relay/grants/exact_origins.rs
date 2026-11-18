use protocol::{PaneOrigin, RelayScope};

use crate::relay::grants::origin_policy::OriginPolicy;

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
    pub fn new(
        core: Vec<String>,
        extension_template: impl Into<String>,
    ) -> Self {
        Self {
            core,
            extension_template: extension_template.into(),
        }
    }
}

impl OriginPolicy for ExactOrigins {
    fn allows(
        &self,
        scope: &RelayScope,
        origin: &str,
    ) -> bool {
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
mod tests {
    use protocol::{ExtensionId, HostId, PaneId, PaneOrigin};

    use super::*;

    fn scope(extension: Option<&str>) -> RelayScope {
        let origin = extension.map_or(PaneOrigin::Core, |id| {
            PaneOrigin::Extension(ExtensionId::new(id).unwrap())
        });
        RelayScope::new(
            HostId::new("local").unwrap(),
            PaneId::new("p1").unwrap(),
            origin,
        )
    }

    fn policy() -> ExactOrigins {
        ExactOrigins::new(
            vec!["crowbar-pane://app".into()],
            "crowbar-ext://{extension}",
        )
    }

    #[test]
    fn a_core_pane_may_present_only_a_core_origin() {
        let core = scope(None);
        assert!(policy().allows(&core, "crowbar-pane://app"));
        for origin in [
            "",
            "null",
            "crowbar-pane://app/",
            "crowbar-pane://app.evil",
            "http://127.0.0.1:1",
            "crowbar-ext://acme",
        ] {
            assert!(!policy().allows(&core, origin), "{origin}");
        }
    }

    #[test]
    fn an_extension_pane_may_present_only_its_own_origin() {
        let acme = scope(Some("acme"));
        assert!(policy().allows(&acme, "crowbar-ext://acme"));
        for origin in [
            "crowbar-ext://other",
            "crowbar-ext://acme2",
            "crowbar-pane://app",
            "null",
        ] {
            assert!(!policy().allows(&acme, origin), "{origin}");
        }
    }
}
