//! The one place a stored or window-sized `f64` becomes the `f32` the toolkit paints with.

use gpui::Pixels;

pub(crate) fn single_precision(value: f64) -> f32 {
    f32::from(Pixels::from(value))
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[(f64, f32)] = &[(0.0, 0.0), (13.5, 13.5), (-2.0, -2.0), (1_920.0, 1_920.0)];

    #[test]
    fn exact_values_survive_the_narrowing() {
        for &(wide, narrow) in CASES {
            assert!(
                (single_precision(wide) - narrow).abs() < f32::EPSILON,
                "{wide}"
            );
        }
    }
}
