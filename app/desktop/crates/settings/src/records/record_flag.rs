//! A switch on a record.

use crate::records::flag_look::FlagLook;

/// An on/off value a record carries, shown as a switch in the column of the same position.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecordFlag {
    /// The id of the flag.
    pub id: String,
    /// The name of the flag for assistive technology and tooltips.
    pub label: String,
    /// Whether the flag is on.
    pub on: bool,
    /// How the flag is drawn.
    pub look: FlagLook,
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keeps_its_fields() {
        let flag = RecordFlag::new("enabled", "Enable Claude Code").on();
        assert_eq!(
            (flag.id.as_str(), flag.label.as_str(), flag.on, flag.look),
            ("enabled", "Enable Claude Code", true, FlagLook::Switch)
        );
        assert_eq!(flag.as_dot().look, FlagLook::Dot);
    }
}
