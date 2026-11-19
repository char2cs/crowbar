//! Piecewise keyframe animations with one easing curve per interval, like CSS `@keyframes`.

use crate::foundation::motion::cubic_bezier::CubicBezier;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Keyframes {
    stops: &'static [(f32, f32)],
    curve: CubicBezier,
}

impl Keyframes {
    pub(crate) const TOAST_SUCCESS: Self = Self::new(
        &[(0.0, 1.0), (0.3, 1.025), (0.6, 0.99), (1.0, 1.0)],
        CubicBezier::TOAST,
    );
    pub(crate) const TOAST_ERROR: Self = Self::new(
        &[
            (0.0, 0.0),
            (0.25, -3.0),
            (0.5, 3.0),
            (0.75, -3.0),
            (1.0, 0.0),
        ],
        CubicBezier::TOAST,
    );

    pub(crate) const fn new(
        stops: &'static [(f32, f32)],
        curve: CubicBezier,
    ) -> Self {
        Self { stops, curve }
    }

    pub(crate) fn sample(
        &self,
        t: f32,
    ) -> f32 {
        let t = t.clamp(0.0, 1.0);
        let segment = self
            .stops
            .windows(2)
            .find(|pair| matches!(pair, [_, (end, _)] if t <= *end));
        let Some([(start, from), (end, to)]) = segment else {
            return self.stops.last().map_or(0.0, |&(_, value)| value);
        };
        let local = (t - start) / (end - start);
        from + (to - from) * self.curve.ease(local)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stops_are_hit_exactly() {
        const CASES: &[(Keyframes, f32, f32)] = &[
            (Keyframes::TOAST_SUCCESS, 0.0, 1.0),
            (Keyframes::TOAST_SUCCESS, 0.3, 1.025),
            (Keyframes::TOAST_SUCCESS, 0.6, 0.99),
            (Keyframes::TOAST_SUCCESS, 1.0, 1.0),
            (Keyframes::TOAST_ERROR, 0.25, -3.0),
            (Keyframes::TOAST_ERROR, 0.5, 3.0),
            (Keyframes::TOAST_ERROR, 0.75, -3.0),
        ];
        for &(keyframes, t, want) in CASES {
            let got = keyframes.sample(t);
            assert!(
                (got - want).abs() < 1e-3,
                "{keyframes:?} at {t}: {got} vs {want}"
            );
        }
    }

    #[test]
    fn intervals_use_the_curve() {
        let midpoint = Keyframes::TOAST_SUCCESS.sample(0.15);
        let eased = 1.0 + 0.025 * CubicBezier::TOAST.ease(0.5);
        assert!((midpoint - eased).abs() < 1e-4, "{midpoint} vs {eased}");
    }
}
