use crate::identity::extension_id::ExtensionId;

/// Who authored a pane: the app itself or one named extension.
///
/// Relay tokens, daemon requests and origin checks are scoped by this value. An extension
/// origin always names its extension and a core origin never does, so a contradictory scope
/// cannot be built.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum PaneOrigin {
    /// Shipped with the app, may use the full daemon API.
    Core,
    /// Provided by the named extension, restricted to that extension's own routes.
    Extension(ExtensionId),
}

impl PaneOrigin {
    /// The extension of an extension origin, `None` for core.
    #[must_use]
    pub fn extension(&self) -> Option<&ExtensionId> {
        match self {
            Self::Core => None,
            Self::Extension(id) => Some(id),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_extension_origin_names_its_extension_and_core_names_none() {
        let id = ExtensionId::new("git").unwrap();
        assert_eq!(PaneOrigin::Core.extension(), None, "core");
        assert_eq!(
            PaneOrigin::Extension(id.clone()).extension(),
            Some(&id),
            "extension"
        );
    }
}
