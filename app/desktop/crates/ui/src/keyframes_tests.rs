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
        (Keyframes::CARET_BLINK, 0.2, 0.0),
        (Keyframes::CARET_BLINK, 0.7, 1.0),
    ];
    for &(keyframes, t, want) in CASES {
        let got = keyframes.sample(t);
        assert!((got - want).abs() < 1e-3, "{keyframes:?} at {t}: {got} vs {want}");
    }
}

#[test]
fn caret_stays_hidden_between_its_hidden_stops() {
    assert!(Keyframes::CARET_BLINK.sample(0.35).abs() < 1e-3);
}

#[test]
fn intervals_use_the_curve() {
    let midpoint = Keyframes::TOAST_SUCCESS.sample(0.15);
    let eased = 1.0 + 0.025 * CubicBezier::TOAST.ease(0.5);
    assert!((midpoint - eased).abs() < 1e-4, "{midpoint} vs {eased}");
}
