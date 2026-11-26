//! One setting in a section.

mod label_style;
mod reset_offer;
mod wording;

pub use label_style::LabelStyle;
pub(crate) use reset_offer::ResetOffer;

use crate::schema::condition::Condition;
use crate::schema::control::Control;
use crate::values::Key;
use crate::values::Value;

/// A label, description and control bound to one setting key.
#[derive(Debug, Clone, PartialEq)]
pub struct Row {
    pub(crate) key: Key,
    pub(crate) label: String,
    pub(crate) description: Option<String>,
    pub(crate) invalid_description: Option<String>,
    pub(crate) worded: Option<(Condition, String)>,
    pub(crate) frozen: Option<Condition>,
    pub(crate) control: Control,
    pub(crate) default: Option<Value>,
    pub(crate) when: Option<Condition>,
    pub(crate) reset: ResetOffer,
    pub(crate) label_style: LabelStyle,
}

impl Row {
    /// The control that edits the value.
    #[must_use]
    pub fn control(&self) -> &Control {
        &self.control
    }

    pub(crate) fn new(
        key: impl Into<Key>,
        label: impl Into<String>,
        control: Control,
    ) -> Self {
        Self {
            key: key.into(),
            label: label.into(),
            description: None,
            invalid_description: None,
            worded: None,
            frozen: None,
            control,
            default: None,
            when: None,
            reset: ResetOffer::Offered,
            label_style: LabelStyle::Label,
        }
    }

    #[must_use]
    pub(crate) fn frozen_while(
        mut self,
        condition: Condition,
    ) -> Self {
        self.frozen = Some(condition);
        self
    }

    #[must_use]
    pub(crate) fn default_value(
        mut self,
        value: impl Into<Value>,
    ) -> Self {
        self.default = Some(value.into());
        self
    }

    #[must_use]
    pub(crate) fn without_reset(mut self) -> Self {
        self.reset = ResetOffer::Withheld;
        self
    }

    #[must_use]
    pub(crate) fn captioned(mut self) -> Self {
        self.label_style = LabelStyle::Caption;
        self
    }

    #[must_use]
    pub(crate) fn when(
        mut self,
        condition: Condition,
    ) -> Self {
        self.when = Some(condition);
        self
    }

    /// The setting this row edits.
    pub fn key(&self) -> &Key {
        &self.key
    }

    /// The label shown to the user.
    pub fn label(&self) -> &str {
        &self.label
    }

    /// How the label is drawn.
    pub fn label_style(&self) -> LabelStyle {
        self.label_style
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_fill_the_row() {
        let row = Row::new("tabSize", "Tab Size", Control::switch())
            .describe("Spaces per tab")
            .describe_invalid("Bad")
            .default_value(2.0)
            .when(Condition::capability("editor"))
            .describe_while((Condition::capability("tabs"), "Fixed"))
            .frozen_while(Condition::capability("tabs"));
        assert_eq!((row.key().as_str(), row.label()), ("tabSize", "Tab Size"));
        assert_eq!(row.description.as_deref(), Some("Spaces per tab"));
        assert_eq!(row.invalid_description.as_deref(), Some("Bad"));
        assert_eq!(row.default, Some(Value::Number(2.0)));
        assert!(row.when.is_some());
        assert_eq!(
            row.worded.as_ref().map(|(_, text)| text.as_str()),
            Some("Fixed")
        );
        assert_eq!(row.frozen, Some(Condition::capability("tabs")));
        assert_eq!(row.label_style(), LabelStyle::Label);
        assert_eq!(row.captioned().label_style(), LabelStyle::Caption);
    }
}
