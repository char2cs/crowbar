use crate::records::flag_look::FlagLook;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecordFlag {
    pub(crate) id: String,
    pub(crate) label: String,
    pub(crate) on: bool,
    pub(crate) look: FlagLook,
}

impl RecordFlag {
    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

    #[must_use]
    pub fn is_on(&self) -> bool {
        self.on
    }

    #[must_use]
    pub fn look(&self) -> FlagLook {
        self.look
    }

    #[must_use]
    pub fn id(&self) -> &str {
        &self.id
    }

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

    #[must_use]
    pub fn as_dot(mut self) -> Self {
        self.look = FlagLook::Dot;
        self
    }

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
