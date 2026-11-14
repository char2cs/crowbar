use crate::error::{Error, Result};
use crate::extension_id::ExtensionId;
use crate::pane_kind::PaneKind;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("pane type id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, '-' | '_' | '.')
    })?;
    let segments: Vec<&str> = value.split('.').collect();
    let well_formed = segments.len() >= 2 && segments.iter().all(|s| !s.is_empty());
    let namespaced = match segments.first() {
        Some(&"core") => true,
        Some(&"ext") => segments.len() >= 3,
        _ => false,
    };
    if well_formed && namespaced {
        return Ok(());
    }
    Err(Error::MalformedId {
        kind: "pane type id",
        reason: "must be `core.<name>` or `ext.<extension>.<name>`",
    })
}

string_id!(
    /// Namespaced pane type: `core.chat` or `ext.<extension>.<pane>`.
    PaneTypeId,
    validate
);

impl PaneTypeId {
    /// Whether the type belongs to the core app or to an extension.
    #[must_use]
    pub fn kind(&self) -> PaneKind {
        if self.as_str().starts_with("ext.") {
            PaneKind::Extension
        } else {
            PaneKind::Core
        }
    }

    /// The extension that owns the type, or `None` for core types.
    #[must_use]
    pub fn extension(&self) -> Option<ExtensionId> {
        let name = self.as_str().strip_prefix("ext.")?.split('.').next()?;
        ExtensionId::new(name).ok()
    }
}

#[cfg(test)]
#[path = "pane_type_id_tests.rs"]
mod tests;
