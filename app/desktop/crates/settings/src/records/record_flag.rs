//! A switch on a record.

use crate::records::flag_look::FlagLook;

/// An on/off value a record carries, shown as a switch in the column of the same position.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecordFlag {
    pub(crate) id: String,
    pub(crate) label: String,
    pub(crate) on: bool,
    pub(crate) look: FlagLook,
}

impl RecordFlag {
    /// The name of the flag for assistive technology and tooltips.
    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

    /// Whether the flag is on.
    #[must_use]
    pub fn is_on(&self) -> bool {
        self.on
    }

    /// How the flag is drawn.
    #[must_use]
    pub fn look(&self) -> FlagLook {
        self.look
    }

    /// The id of the flag.
    #[must_use]
    pub fn id(&self) -> &str {
        &self.id
    }

    /// A flag `id`, off; `label` names it for assistive technology and tooltips.
    pub fn new(
        id: impl Into<String>,
        label: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            on: false,
            look: FlagLook::Switch,
        }
    }

    /// The same flag, drawn as a small status dot the user cannot toggle.
    #[must_use]
    pub fn as_dot(mut self) -> Self {
        self.look = FlagLook::Dot;
        self
    }

    /// The same flag, switched on.
    #[must_use]
    pub fn on(mut self) -> Self {
        self.on = true;
        self
    }
}
