//! A switch on a record.

/// An on/off value a record carries, shown as a switch in the column of the same position.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecordFlag {
    pub(crate) id: String,
    pub(crate) label: String,
    pub(crate) on: bool,
}

impl RecordFlag {
    /// A flag `id`, off; `label` names it for assistive technology and tooltips.
    pub fn new(id: impl Into<String>, label: impl Into<String>) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            on: false,
        }
    }

    /// The same flag, switched on.
    #[must_use]
    pub fn on(mut self) -> Self {
        self.on = true;
        self
    }
}

#[cfg(test)]
#[path = "record_flag_tests.rs"]
mod tests;
