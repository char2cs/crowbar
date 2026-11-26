//! Bounds, step and text form of a number field.

use crate::schema::control::nudge::Nudge;
use crate::schema::control::on_load::OnLoad;

/// The allowed numbers of a field: `min..=max` in increments of `step`.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct NumberRange {
    pub(crate) min: f64,
    pub(crate) max: f64,
    pub(crate) step: f64,
    snaps: bool,
    on_load: OnLoad,
}

impl NumberRange {
    /// The smallest allowed number.
    #[must_use]
    pub fn min(&self) -> f64 {
        self.min
    }

    /// The largest allowed number.
    #[must_use]
    pub fn max(&self) -> f64 {
        self.max
    }

    pub(crate) fn new(
        min: f64,
        max: f64,
        step: f64,
    ) -> Self {
        Self {
            min,
            max,
            step,
            snaps: false,
            on_load: OnLoad::Kept,
        }
    }

    #[must_use]
    pub(crate) fn snapping(mut self) -> Self {
        self.snaps = true;
        self
    }

    #[must_use]
    pub(crate) fn loaded_as(
        mut self,
        on_load: OnLoad,
    ) -> Self {
        self.on_load = on_load;
        self
    }

    pub(crate) fn precision(&self) -> usize {
        let text = format!("{}", self.step);
        text.split_once('.')
            .map_or(0, |(_, decimals)| decimals.len())
    }

    fn snap(
        &self,
        value: f64,
    ) -> f64 {
        if !self.snaps {
            return value;
        }
        ((value - self.min) / self.step)
            .round()
            .mul_add(self.step, self.min)
    }

    pub(crate) fn normalize(
        &self,
        value: f64,
    ) -> f64 {
        if !value.is_finite() {
            return self.min;
        }
        let snapped = self.snap(value);
        let scale = 10_f64.powi(i32::try_from(self.precision()).unwrap_or(0));
        ((snapped * scale).round() / scale).clamp(self.min, self.max)
    }

    pub(crate) fn loaded(
        &self,
        value: f64,
    ) -> f64 {
        match self.on_load {
            OnLoad::Kept => value,
            OnLoad::Normalized => self.normalize(value),
            OnLoad::Replaced(legacy) => legacy.loaded(value),
        }
    }

    /// The number typed in `text`, if it is one.
    pub fn parse(
        &self,
        text: &str,
    ) -> Option<f64> {
        text.trim()
            .parse::<f64>()
            .ok()
            .filter(|number| number.is_finite())
    }

    pub(crate) fn format(
        &self,
        value: f64,
    ) -> String {
        let fixed = format!("{value:.precision$}", precision = self.precision());
        if !fixed.contains('.') {
            return fixed;
        }
        fixed.trim_end_matches('0').trim_end_matches('.').to_owned()
    }

    pub(crate) fn stepped(
        &self,
        current: f64,
        nudge: Nudge,
    ) -> f64 {
        match nudge {
            Nudge::Up => self.normalize(current + self.step),
            Nudge::Down => self.normalize(current - self.step),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn near(
        left: f64,
        right: f64,
    ) -> bool {
        (left - right).abs() < 1e-9
    }

    #[test]
    fn normalize_clamps_rounds_and_snaps() {
        let font = NumberRange::new(10.0, 24.0, 0.5).snapping();
        let indent = NumberRange::new(8.0, 32.0, 1.0);
        let scrollback = NumberRange::new(1000.0, 100_000.0, 1000.0);
        assert!(near(font.normalize(15.3), 15.5), "snaps to the half pixel");
        assert!(near(font.normalize(99.0), 24.0), "clamps above");
        assert!(near(indent.normalize(7.4), 8.0), "clamps below");
        assert!(
            near(indent.normalize(16.4), 16.0),
            "rounds to whole numbers"
        );
        assert!(
            near(scrollback.normalize(1500.0), 1500.0),
            "large steps do not snap"
        );
        assert!(
            near(indent.normalize(f64::NAN), 8.0),
            "non-finite falls to the minimum"
        );
    }

    #[test]
    fn format_uses_step_precision_without_trailing_zeros() {
        type Case = (f64, f64, f64, &'static str);
        const CASES: &[Case] = &[
            (10.0, 24.0, 0.5, "15"),
            (10.0, 24.0, 0.5, "15"),
            (1.0, 2.0, 0.1, "1.4"),
            (8.0, 32.0, 1.0, "14"),
        ];
        let values = [15.0, 15.0, 1.4, 14.0];
        for (&(min, max, step, expected), value) in CASES.iter().zip(values) {
            assert_eq!(
                NumberRange::new(min, max, step).format(value),
                expected,
                "step {step}"
            );
        }
        assert_eq!(NumberRange::new(10.0, 24.0, 0.5).format(15.5), "15.5");
    }

    #[test]
    fn parse_accepts_numbers_only() {
        let range = NumberRange::new(0.0, 10.0, 1.0);
        assert_eq!(range.parse(" 4 "), Some(4.0));
        for text in ["", "-", "abc", "NaN", "inf"] {
            assert_eq!(range.parse(text), None, "{text:?}");
        }
    }

    #[test]
    fn stepping_stops_at_the_bounds() {
        let range = NumberRange::new(1.0, 6.0, 1.0);
        assert!(near(range.stepped(6.0, Nudge::Up), 6.0), "max holds");
        assert!(near(range.stepped(1.0, Nudge::Down), 1.0), "min holds");
        assert!(near(range.stepped(3.0, Nudge::Up), 4.0), "one step up");
    }
}
