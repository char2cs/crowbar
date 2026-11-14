use super::*;

const CODES: &[&str] = &[
    "layout_newer",
    "layout_unreadable",
    "ledger_corrupt",
    "io_failed",
    "plan_failed",
    "preflight_failed",
    "step_failed",
    "verify_failed",
    "no_implementation",
];

#[test]
fn every_known_code_round_trips_and_explains() {
    for &code in CODES {
        let reason = HaltReason::from_code(Some(code));
        assert!(!matches!(reason, HaltReason::Other(_)), "{code}");
        assert_eq!(reason.code(), Some(code));
        assert!(reason.explanation().len() > 20, "{code}");
    }
}

#[test]
fn an_unknown_or_missing_code_is_kept_for_display() {
    assert_eq!(
        HaltReason::from_code(Some("brand_new")).code(),
        Some("brand_new")
    );
    assert_eq!(HaltReason::from_code(None).code(), None);
    assert!(matches!(
        HaltReason::from_code(None),
        HaltReason::Other(None)
    ));
}
