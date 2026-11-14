use super::*;

fn near(left: f64, right: f64) -> bool {
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
