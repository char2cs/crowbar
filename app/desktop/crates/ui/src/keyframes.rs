//! Piecewise keyframe animations with one easing curve per interval, like CSS `@keyframes`.

use crate::cubic_bezier::CubicBezier;

/// A list of `(offset, value)` stops, offsets rising from 0 to 1.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Keyframes {
    stops: &'static [(f32, f32)],
    curve: CubicBezier,
}

impl Keyframes {
    /// Toast success: scale 1, 1.025 at 30%, 0.99 at 60%, 1.
    pub const TOAST_SUCCESS: Self = Self::new(
        &[(0.0, 1.0), (0.3, 1.025), (0.6, 0.99), (1.0, 1.0)],
        CubicBezier::TOAST,
    );
    /// Toast error: translateX 0, -3px, 3px, -3px, 0 at quarter steps.
    pub const TOAST_ERROR: Self = Self::new(
        &[(0.0, 0.0), (0.25, -3.0), (0.5, 3.0), (0.75, -3.0), (1.0, 0.0)],
        CubicBezier::TOAST,
    );
    /// Caret blink opacity: 1 at 0, 70 and 100%, 0 at 20 and 50%, eased out.
    pub const CARET_BLINK: Self = Self::new(
        &[(0.0, 1.0), (0.2, 0.0), (0.5, 0.0), (0.7, 1.0), (1.0, 1.0)],
        CubicBezier::EASE_OUT,
    );

    /// Builds keyframes; `stops` must be sorted by offset and span 0 to 1.
    pub const fn new(stops: &'static [(f32, f32)], curve: CubicBezier) -> Self {
        Self { stops, curve }
    }

    /// Value at progress `t` in 0 to 1.
    pub fn sample(&self, t: f32) -> f32 {
        let t = t.clamp(0.0, 1.0);
        let segment = self.stops.windows(2).find(|pair| matches!(pair, [_, (end, _)] if t <= *end));
        let Some([(start, from), (end, to)]) = segment else {
            return self.stops.last().map_or(0.0, |&(_, value)| value);
        };
        let local = (t - start) / (end - start);
        from + (to - from) * self.curve.ease(local)
    }
}

#[cfg(test)]
#[path = "keyframes_tests.rs"]
mod tests;
