use protocol::{ExtensionId, PaneKind, PaneTypeId};

/// Who a web view acts for, derived from the pane type it was opened for.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Origin {
    /// Core or extension.
    pub kind: PaneKind,
    /// The owning extension for extension panes.
    pub extension: Option<ExtensionId>,
}

impl Origin {
    /// The origin of panes of `type_id`.
    #[must_use]
    pub fn of(type_id: &PaneTypeId) -> Self {
        Self {
            kind: type_id.kind(),
            extension: type_id.extension(),
        }
    }
}

#[cfg(test)]
#[path = "origin_tests.rs"]
mod tests;
