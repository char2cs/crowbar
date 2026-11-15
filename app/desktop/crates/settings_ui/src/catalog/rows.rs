//! Short constructors for the rows of the built-in pages.

use crate::control::Control;
use crate::row::Row;
use crate::value::Value;

pub(super) fn row(
    (key, label): (&str, &str),
    description: &str,
    control: Control,
    default: impl Into<Value>,
) -> Row {
    Row::new(key, label, control)
        .describe(description)
        .default_value(default)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_carry_label_description_and_default() {
        let built = row(
            ("wordWrap", "Word Wrap"),
            "Wrap lines",
            Control::switch(),
            Value::Bool(false),
        );
        assert_eq!(
            (built.key().as_str(), built.label()),
            ("wordWrap", "Word Wrap")
        );
        assert_eq!(built.description.as_deref(), Some("Wrap lines"));
        assert_eq!(built.default, Some(Value::Bool(false)));
    }
}
