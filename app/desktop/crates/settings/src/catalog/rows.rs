//! Short constructors for the rows of the built-in pages.

pub(super) mod number_setting;

use crate::schema::Control;
use crate::schema::Row;
use crate::values::Value;

pub(super) fn row(
    names: (&str, &str),
    description: &str,
    control: Control,
    default: impl Into<Value>,
) -> Row {
    let (key, label) = names;
    Row::new(key, label, control)
        .describe(description)
        .default_value(default)
}
