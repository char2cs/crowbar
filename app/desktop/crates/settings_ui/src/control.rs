//! The widget of a row and how its value maps to text and back.

use crate::button_spec::ButtonSpec;
use crate::choice_source::ChoiceSource;
use crate::control_width::ControlWidth;
use crate::issue::Issue;
use crate::number_range::NumberRange;
use crate::validator::Validator;
use crate::value::Value;

/// What a row shows on its right: the schema's fixed vocabulary of controls.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Control {
    /// An on/off switch; `invert` flips it against the stored flag.
    Switch {
        /// Whether the switch reads opposite to the stored value.
        invert: bool,
    },
    /// A dropdown of options.
    Select {
        /// Where the options come from.
        source: ChoiceSource,
        /// Width class of the trigger.
        width: ControlWidth,
        /// Shown while nothing matches the stored value.
        placeholder: Option<String>,
        /// Tooltip text of an info icon after the trigger.
        hint: Option<String>,
    },
    /// A stepper with a typed field between minus and plus buttons.
    Number {
        /// Allowed numbers.
        range: NumberRange,
        /// Width class of the whole stepper.
        width: ControlWidth,
    },
    /// A single-line text field.
    Text {
        /// Shown while empty.
        placeholder: String,
        /// Width class.
        width: ControlWidth,
        /// Check that flags invalid text.
        validator: Option<Validator>,
    },
    /// A comma-separated list in a text field.
    TextList {
        /// Shown while empty.
        placeholder: String,
        /// Width class.
        width: ControlWidth,
    },
    /// A value shown as text, never edited.
    Readout {
        /// Whether the text uses the monospace family.
        mono: bool,
    },
    /// A button.
    Button(ButtonSpec),
}

impl Control {
    /// A switch.
    pub fn switch() -> Self {
        Self::Switch { invert: false }
    }

    /// A switch that reads opposite to the stored flag.
    pub fn inverted_switch() -> Self {
        Self::Switch { invert: true }
    }

    /// A select over fixed `(value, label)` options.
    pub fn select(
        options: &[(&str, &str)],
        width: ControlWidth,
    ) -> Self {
        Self::select_from(ChoiceSource::fixed(options), width)
    }

    /// A select over the options the backend lists as `provider`.
    pub fn provided_select(
        provider: &str,
        width: ControlWidth,
    ) -> Self {
        Self::select_from(ChoiceSource::provided(provider), width)
    }

    /// A select over `source`.
    pub fn select_from(
        source: ChoiceSource,
        width: ControlWidth,
    ) -> Self {
        Self::Select {
            source,
            width,
            placeholder: None,
            hint: None,
        }
    }

    /// A number stepper.
    pub fn number(
        range: NumberRange,
        width: ControlWidth,
    ) -> Self {
        Self::Number { range, width }
    }

    /// A text field.
    pub fn text(
        placeholder: &str,
        width: ControlWidth,
    ) -> Self {
        Self::Text {
            placeholder: placeholder.to_owned(),
            width,
            validator: None,
        }
    }

    /// A comma-separated list field.
    pub fn text_list(
        placeholder: &str,
        width: ControlWidth,
    ) -> Self {
        Self::TextList {
            placeholder: placeholder.to_owned(),
            width,
        }
    }

    /// Flags text that fails `validator`.
    #[must_use]
    pub fn validated(
        mut self,
        validator: Validator,
    ) -> Self {
        if let Self::Text {
            validator: slot, ..
        } = &mut self
        {
            *slot = Some(validator);
        }
        self
    }

    /// Text a select shows while no option matches.
    #[must_use]
    pub fn placeholder(
        mut self,
        text: &str,
    ) -> Self {
        if let Self::Select { placeholder, .. } = &mut self {
            *placeholder = Some(text.to_owned());
        }
        self
    }

    /// Adds an info icon with `text` as its tooltip after a select.
    #[must_use]
    pub fn hint(
        mut self,
        text: &str,
    ) -> Self {
        if let Self::Select { hint, .. } = &mut self {
            *hint = Some(text.to_owned());
        }
        self
    }

    pub(crate) fn text_of(
        &self,
        value: Option<&Value>,
    ) -> String {
        match (self, value) {
            (Self::Number { range, .. }, Some(Value::Number(number))) => range.format(*number),
            (Self::Text { .. }, Some(Value::Text(text))) => text.clone(),
            (Self::TextList { .. }, Some(Value::List(items))) => items.join(", "),
            _ => String::new(),
        }
    }

    pub(crate) fn parse(
        &self,
        text: &str,
    ) -> Result<Value, Issue> {
        match self {
            Self::Number { range, .. } => range
                .parse(text)
                .map(|number| Value::Number(range.normalize(number)))
                .ok_or(Issue::NotANumber),
            Self::TextList { .. } => Ok(Value::List(split_list(text))),
            _ => Ok(Value::Text(text.to_owned())),
        }
    }

    pub(crate) fn issue(
        &self,
        text: &str,
    ) -> Option<Issue> {
        match self {
            Self::Text {
                validator: Some(validator),
                ..
            } => validator.check(text).err(),
            Self::Number { range, .. } if range.parse(text).is_none() => Some(Issue::NotANumber),
            _ => None,
        }
    }
}

fn split_list(text: &str) -> Vec<String> {
    text.split(',')
        .map(str::trim)
        .filter(|item| !item.is_empty())
        .map(str::to_owned)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{Case, run_cases};

    fn number() -> Control {
        Control::number(
            NumberRange::new(8.0, 32.0, 1.0),
            ControlWidth::NumberCompact,
        )
    }

    fn text_round_trips_through_the_value() {
        let list = Control::text_list("*.log", ControlWidth::Text);
        let value = Value::List(vec!["a".to_owned(), "b/".to_owned()]);
        assert_eq!(list.text_of(Some(&value)), "a, b/");
        assert_eq!(list.parse(" a , ,b/ ,"), Ok(value));
        assert_eq!(number().text_of(Some(&Value::Number(14.0))), "14");
        assert_eq!(number().text_of(None), "");
    }

    fn numbers_are_normalised_when_parsed() {
        assert_eq!(number().parse("99"), Ok(Value::Number(32.0)));
        assert_eq!(number().parse("x"), Err(Issue::NotANumber));
        assert_eq!(number().issue("x"), Some(Issue::NotANumber));
        assert_eq!(number().issue("12"), None);
    }

    fn text_is_kept_as_typed_but_flagged() {
        let url = Control::text("https://kagi.com/search?q=%s", ControlWidth::TextWide)
            .validated(Validator::SearchTemplate);
        assert_eq!(url.parse("nope"), Ok(Value::Text("nope".to_owned())));
        assert_eq!(url.issue("nope"), Some(Issue::BadSearchTemplate));
        assert_eq!(url.issue(""), None);
    }

    fn modifiers_apply_only_to_their_control() {
        let select = Control::provided_select("themes", ControlWidth::Wide)
            .placeholder("Select font...")
            .hint("Install a Nerd Font");
        assert!(matches!(
            select,
            Control::Select {
                placeholder: Some(_),
                hint: Some(_),
                ..
            }
        ));
        assert_eq!(
            Control::switch().placeholder("x").hint("y"),
            Control::switch()
        );
    }

    const CASES: &[Case] = &[
        (
            "text round trips through the value",
            text_round_trips_through_the_value,
        ),
        (
            "numbers are normalised when parsed",
            numbers_are_normalised_when_parsed,
        ),
        (
            "text is kept as typed but flagged",
            text_is_kept_as_typed_but_flagged,
        ),
        (
            "modifiers apply only to their control",
            modifiers_apply_only_to_their_control,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
