use crate::identity::extension_id::ExtensionId;

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum PaneOrigin {
    Core,
    Extension(ExtensionId),
}

impl PaneOrigin {
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
