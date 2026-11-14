use std::time::{Duration, Instant};

use ui::Motion;

use super::*;
use crate::test_support::pane_id;

const FULL: Duration = Duration::from_millis(150);

#[test]
fn a_pane_fades_in_over_150_ms_and_out_again() {
    let start = Instant::now();
    let mut fades = PaneFades::default();
    fades.show(&pane_id("a"), start, Motion::default());
    assert_eq!(
        fades.opacity(&pane_id("a"), start, Motion::default()),
        0.0,
        "starts clear"
    );
    assert_eq!(
        fades.opacity(&pane_id("a"), start + FULL, Motion::default()),
        1.0,
        "ends shown"
    );
    fades.hide_all_but(None, start + FULL, Motion::default());
    assert_eq!(
        fades.opacity(&pane_id("a"), start + FULL * 2, Motion::default()),
        0.0,
        "gone"
    );
}

#[test]
fn an_unknown_pane_is_invisible() {
    let fades = PaneFades::default();
    assert_eq!(
        fades.opacity(&pane_id("x"), Instant::now(), Motion::default()),
        0.0,
        "invisible"
    );
}

#[test]
fn hide_all_but_keeps_only_the_named_pane() {
    let start = Instant::now();
    let mut fades = PaneFades::default();
    fades.show(&pane_id("a"), start, Motion::default());
    fades.show(&pane_id("b"), start, Motion::default());
    let later = start + FULL;
    fades.hide_all_but(Some(&pane_id("a")), later, Motion::default());
    let end = later + FULL;
    assert_eq!(
        fades.opacity(&pane_id("a"), end, Motion::default()),
        1.0,
        "a stays"
    );
    assert_eq!(
        fades.opacity(&pane_id("b"), end, Motion::default()),
        0.0,
        "b hides"
    );
}

#[test]
fn retain_forgets_closed_panes() {
    let start = Instant::now();
    let mut fades = PaneFades::default();
    fades.show(&pane_id("a"), start, Motion::default());
    fades.retain(&[pane_id("b")]);
    assert!(
        !fades.is_running(start, Motion::default()),
        "nothing left to run"
    );
}
