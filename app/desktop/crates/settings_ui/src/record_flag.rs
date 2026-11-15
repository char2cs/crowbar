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
    pub fn new(
        id: impl Into<String>,
        label: impl Into<String>,
    ) -> Self {
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
mod tests {
    use super::*;

    #[test]
    fn keeps_its_fields() {
        let flag = RecordFlag::new("enabled", "Enable Claude Code").on();
        assert_eq!(
            (flag.id.as_str(), flag.label.as_str(), flag.on),
            ("enabled", "Enable Claude Code", true)
        );
    }
}
