mod action;
mod button_face;
mod confirm;

pub use action::Action;
pub use button_face::ButtonFace;
pub use confirm::Confirm;

use crate::schema::condition::Condition;

#[derive(Debug, Clone, PartialEq)]
pub struct ButtonSpec {
    pub(crate) label: String,
    pub(crate) icon: Option<String>,
    pub(crate) action: Action,
    pub(crate) confirm: Option<Confirm>,
    pub(crate) enabled: Option<Condition>,
    pub(crate) disabled_note: Option<String>,
    pub(crate) note: Option<String>,
    pub(crate) face: ButtonFace,
}

impl ButtonSpec {
    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

    #[must_use]
    pub fn icon_stem(&self) -> Option<&str> {
        self.icon.as_deref()
    }

    #[must_use]
    pub fn action(&self) -> &Action {
        &self.action
    }

    pub(crate) fn new(
        label: impl Into<String>,
        action: Action,
    ) -> Self {
        Self {
            label: label.into(),
            icon: None,
            action,
            confirm: None,
            enabled: None,
            disabled_note: None,
            note: None,
            face: ButtonFace::Button,
        }
    }

    #[must_use]
    pub(crate) fn icon(
        mut self,
        stem: impl Into<String>,
    ) -> Self {
        self.icon = Some(stem.into());
        self
    }

    #[must_use]
    pub(crate) fn confirm(
        mut self,
        confirm: Confirm,
    ) -> Self {
        self.confirm = Some(confirm);
        self
    }

    #[must_use]
    pub(crate) fn enabled_when(
        mut self,
        condition: Condition,
    ) -> Self {
        self.enabled = Some(condition);
        self
    }

    #[must_use]
    pub fn note(&self) -> Option<&str> {
        self.note.as_deref()
    }

    #[must_use]
    pub fn face(&self) -> ButtonFace {
        self.face
    }

    #[must_use]
    pub(crate) fn faced(
        mut self,
        face: ButtonFace,
    ) -> Self {
        self.face = face;
        self
    }

    #[must_use]
    pub(crate) fn noted(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.note = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn disabled_note(
        mut self,
        note: impl Into<String>,
    ) -> Self {
        self.disabled_note = Some(note.into());
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_fill_the_optional_parts() {
        let spec = ButtonSpec::new("Clear data", Action::backend("browser.clearData"))
            .icon("trash")
            .confirm(Confirm::new("Clear?", "Sure?", "Clear data"))
            .enabled_when(Condition::capability("browser.clearData"))
            .disabled_note("Not available")
            .noted("Help")
            .faced(ButtonFace::Label);
        assert_eq!(spec.icon.as_deref(), Some("trash"));
        assert!(spec.confirm.is_some() && spec.enabled.is_some());
        assert_eq!(spec.disabled_note.as_deref(), Some("Not available"));
        assert_eq!(spec.note(), Some("Help"));
        assert_eq!(spec.face(), ButtonFace::Label);
    }
}
