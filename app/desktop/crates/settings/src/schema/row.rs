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
        let rule = Condition::capability("tabs");
        let row = Row::new("tabSize", "Tab Size", Control::switch())
            .default_value(2.0)
            .when(rule.clone())
            .frozen_while(rule.clone());
        assert_eq!(row.default, Some(Value::Number(2.0)));
        assert_eq!(
            (row.when.as_ref(), row.frozen.as_ref()),
            (Some(&rule), Some(&rule))
        );
        assert_eq!(row.captioned().label_style(), LabelStyle::Caption);
    }
}
