//! Bounds, step and text form of a number field.

use crate::nudge::Nudge;

/// The allowed numbers of a field: `min..=max` in increments of `step`.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct NumberRange {
    pub(crate) min: f64,
    pub(crate) max: f64,
    pub(crate) step: f64,
    snaps: bool,
}

impl NumberRange {
    /// A range; typed values are rounded to the precision of `step` and clamped.
    pub fn new(min: f64, max: f64, step: f64) -> Self {
        Self {
            min,
            max,
            step,
            snaps: false,
        }
    }

    /// Typed values also snap to the nearest multiple of the step.
    #[must_use]
    pub fn snapping(mut self) -> Self {
        self.snaps = true;
        self
    }

    /// Decimal places the step needs.
    pub fn precision(&self) -> usize {
        let text = format!("{}", self.step);
        text.split_once('.')
            .map_or(0, |(_, decimals)| decimals.len())
    }

    /// `value` rounded, snapped and clamped into the range.
    pub fn normalize(&self, value: f64) -> f64 {
        if !value.is_finite() {
            return self.min;
        }
        let snapped = if self.snaps {
            ((value - self.min) / self.step)
                .round()
                .mul_add(self.step, self.min)
        } else {
            value
        };
        let scale = 10_f64.powi(i32::try_from(self.precision()).unwrap_or(0));
        ((snapped * scale).round() / scale).clamp(self.min, self.max)
    }

    /// The number typed in `text`, if it is one.
    pub fn parse(&self, text: &str) -> Option<f64> {
        text.trim()
            .parse::<f64>()
            .ok()
            .filter(|number| number.is_finite())
    }

    /// `value` as shown in the field: step precision, no trailing zeros.
    pub fn format(&self, value: f64) -> String {
        let fixed = format!("{value:.precision$}", precision = self.precision());
        if !fixed.contains('.') {
            return fixed;
        }
        fixed.trim_end_matches('0').trim_end_matches('.').to_owned()
    }

    /// `current` moved one step.
    pub fn stepped(&self, current: f64, nudge: Nudge) -> f64 {
        match nudge {
            Nudge::Up => self.normalize(current + self.step),
            Nudge::Down => self.normalize(current - self.step),
        }
    }
}

#[cfg(test)]
#[path = "number_range_tests.rs"]
mod tests;
