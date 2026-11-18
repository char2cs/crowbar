//! CSS `cubic-bezier()` timing functions.

const NEWTON_STEPS: usize = 8;
const BISECTION_STEPS: usize = 40;
const EPSILON: f32 = 1e-6;
const BERNSTEIN_3: f32 = 3.0;
const BERNSTEIN_6: f32 = 6.0;

/// A CSS cubic-bezier easing curve through (0,0) and (1,1) with two control points.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CubicBezier {
    x1: f32,
    y1: f32,
    x2: f32,
    y2: f32,
}

impl CubicBezier {
    /// Tailwind's default transition curve, `cubic-bezier(0.4, 0, 0.2, 1)`.
    pub const DEFAULT: Self = Self::new(0.4, 0.0, 0.2, 1.0);
    /// CSS `ease`, `cubic-bezier(0.25, 0.1, 0.25, 1)`.
    pub const EASE: Self = Self::new(0.25, 0.1, 0.25, 1.0);
    /// CSS `ease-out` as Tailwind defines it, `cubic-bezier(0, 0, 0.2, 1)`.
    pub const EASE_OUT: Self = Self::new(0.0, 0.0, 0.2, 1.0);
    /// The toast keyframe curve, `cubic-bezier(0.5, 1, 0.89, 1)`.
    pub const TOAST: Self = Self::new(0.5, 1.0, 0.89, 1.0);
    /// The identity curve.
    pub const LINEAR: Self = Self::new(0.0, 0.0, 1.0, 1.0);

    /// Builds the curve from the control points of CSS `cubic-bezier(x1, y1, x2, y2)`.
    pub const fn new(
        x1: f32,
        y1: f32,
        x2: f32,
        y2: f32,
    ) -> Self {
        Self { x1, y1, x2, y2 }
    }

    /// Output progress for input progress `t` in 0 to 1; values outside are clamped.
    pub fn ease(
        &self,
        t: f32,
    ) -> f32 {
        let t = t.clamp(0.0, 1.0);
        let parameter = self.parameter_for(t);
        sample(self.y1, self.y2, parameter)
    }

    fn parameter_for(
        &self,
        x: f32,
    ) -> f32 {
        self.newton(x).unwrap_or_else(|| self.bisect(x))
    }

    fn newton(
        &self,
        x: f32,
    ) -> Option<f32> {
        match (0..NEWTON_STEPS).try_fold(x, |guess, _| self.step(guess, x)) {
            Ok(_) => None,
            Err(solved) => solved,
        }
    }

    /// One Newton iteration: `Ok` with the next guess, `Err(Some)` once solved, `Err(None)` when the slope vanishes.
    fn step(
        &self,
        guess: f32,
        x: f32,
    ) -> Result<f32, Option<f32>> {
        let error = sample(self.x1, self.x2, guess) - x;
        if error.abs() < EPSILON {
            return Err(Some(guess));
        }
        let slope = slope(self.x1, self.x2, guess);
        if slope.abs() < EPSILON {
            return Err(None);
        }
        Ok(guess - error / slope)
    }

    fn bisect(
        &self,
        x: f32,
    ) -> f32 {
        let mut bounds = (0.0_f32, 1.0_f32);
        for _ in 0..BISECTION_STEPS {
            bounds = self.narrowed(bounds, x);
        }
        bounds.0.midpoint(bounds.1)
    }

    fn narrowed(
        &self,
        (low, high): (f32, f32),
        x: f32,
    ) -> (f32, f32) {
        let middle = low.midpoint(high);
        if sample(self.x1, self.x2, middle) < x {
            return (middle, high);
        }
        (low, middle)
    }
}

fn sample(
    p1: f32,
    p2: f32,
    s: f32,
) -> f32 {
    let inverse = 1.0 - s;
    BERNSTEIN_3 * inverse * inverse * s * p1 + BERNSTEIN_3 * inverse * s * s * p2 + s * s * s
}

fn slope(
    p1: f32,
    p2: f32,
    s: f32,
) -> f32 {
    let inverse = 1.0 - s;
    BERNSTEIN_3 * inverse * inverse * p1
        + BERNSTEIN_6 * inverse * s * (p2 - p1)
        + BERNSTEIN_3 * s * s * (1.0 - p2)
}

#[cfg(test)]
mod tests {
    use super::*;

    const SAMPLES: [f32; 5] = [0.1, 0.25, 0.5, 0.75, 0.9];

    /// Reference values from an independent 80-step bisection of the CSS definition.
    const CASES: &[(CubicBezier, [f32; 5])] = &[
        (
            CubicBezier::DEFAULT,
            [0.02586, 0.23659, 0.77556, 0.95937, 0.99435],
        ),
        (
            CubicBezier::EASE,
            [0.0948, 0.40851, 0.8024, 0.96046, 0.99432],
        ),
        (
            CubicBezier::EASE_OUT,
            [0.30385, 0.57757, 0.83925, 0.96422, 0.9946],
        ),
        (
            CubicBezier::TOAST,
            [0.18969, 0.43627, 0.74873, 0.93885, 0.99167],
        ),
    ];

    #[test]
    fn curves_match_the_css_definition() {
        for (curve, expected) in CASES {
            let worst = SAMPLES
                .iter()
                .zip(expected)
                .map(|(t, want)| (curve.ease(*t) - want).abs())
                .fold(0.0, f32::max);
            assert!(worst < 2e-4, "{curve:?}: worst deviation {worst}");
        }
    }

    #[test]
    fn endpoints_are_fixed_and_input_is_clamped() {
        for (curve, _) in CASES {
            assert!(curve.ease(0.0).abs() < 1e-5, "{curve:?} start");
            assert!((curve.ease(1.0) - 1.0).abs() < 1e-5, "{curve:?} end");
            assert!((curve.ease(2.0) - 1.0).abs() < 1e-5, "{curve:?} clamp high");
            assert!(curve.ease(-1.0).abs() < 1e-5, "{curve:?} clamp low");
        }
    }

    #[test]
    fn linear_is_identity() {
        for t in SAMPLES {
            assert!((CubicBezier::LINEAR.ease(t) - t).abs() < 1e-4, "t {t}");
        }
    }
}
