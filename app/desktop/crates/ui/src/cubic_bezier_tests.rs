use super::*;

const SAMPLES: [f32; 5] = [0.1, 0.25, 0.5, 0.75, 0.9];

/// Reference values from an independent 80-step bisection of the CSS definition.
const CASES: &[(CubicBezier, [f32; 5])] = &[
    (CubicBezier::DEFAULT, [0.02586, 0.23659, 0.77556, 0.95937, 0.99435]),
    (CubicBezier::EASE, [0.0948, 0.40851, 0.8024, 0.96046, 0.99432]),
    (CubicBezier::EASE_OUT, [0.30385, 0.57757, 0.83925, 0.96422, 0.9946]),
    (CubicBezier::TOAST, [0.18969, 0.43627, 0.74873, 0.93885, 0.99167]),
];

#[test]
fn curves_match_the_css_definition() {
    for (curve, expected) in CASES {
        for (t, want) in SAMPLES.iter().zip(expected) {
            let got = curve.ease(*t);
            assert!((got - want).abs() < 2e-4, "{curve:?} at {t}: {got} vs {want}");
        }
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
