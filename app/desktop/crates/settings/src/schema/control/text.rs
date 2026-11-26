//! How a control's value maps to the text in its field and back.

use crate::schema::control::Control;
use crate::schema::control::issue::Issue;
use crate::values::Value;

impl Control {
    pub(crate) fn text_of(
        &self,
        value: Option<&Value>,
    ) -> String {
        match (self, value.map(|value| self.loaded(value))) {
            (Self::Number { range, .. }, Some(Value::Number(number))) => range.format(number),
            (Self::Text { .. } | Self::Readout { .. }, Some(Value::Text(text))) => text,
            (Self::TextList { .. }, Some(Value::List(items))) => items.join(", "),
            _ => String::new(),
        }
    }

    pub(crate) fn loaded(
        &self,
        value: &Value,
    ) -> Value {
        match (self, value) {
            (Self::Number { range, .. }, Value::Number(number)) => {
                Value::Number(range.loaded(*number))
            },
            _ => value.clone(),
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
    use crate::schema::control::control_width::ControlWidth;
    use crate::schema::control::number_range::NumberRange;
    use crate::schema::control::validator::Validator;
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

    fn a_readout_shows_the_text_it_holds() {
        let readout = Control::Readout { mono: true };
        let text = Value::Text("192.168.1.4:3737".to_owned());
        assert_eq!(readout.text_of(Some(&text)), "192.168.1.4:3737");
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
            "a readout shows the text it holds",
            a_readout_shows_the_text_it_holds,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
