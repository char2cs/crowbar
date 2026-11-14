//! Short constructors for the rows of the built-in pages.

use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::number_range::NumberRange;
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

pub(super) fn number((min, max, step): (f64, f64, f64), width: ControlWidth) -> Control {
    Control::number(NumberRange::new(min, max, step), width)
}

pub(super) fn snapped((min, max, step): (f64, f64, f64), width: ControlWidth) -> Control {
    Control::number(NumberRange::new(min, max, step).snapping(), width)
}

#[cfg(test)]
#[path = "rows_tests.rs"]
mod tests;
