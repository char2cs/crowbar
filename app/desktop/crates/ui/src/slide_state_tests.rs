use std::time::Duration;

use gpui::{AppContext as _, TestAppContext};

use super::*;
use crate::cubic_bezier::CubicBezier;

fn tween() -> Tween {
    Tween::new(Duration::from_millis(200), CubicBezier::LINEAR)
}

#[gpui::test]
fn slides_between_slots_and_settles(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(0));
    state.update(cx, |s, cx| s.go_to(2, tween(), cx));
    let start = Instant::now();
    let (mid, settled) = state.read_with(cx, |s, _| {
        (
            s.at(
                tween(),
                Motion::default(),
                start + Duration::from_millis(100),
            ),
            s.at(
                tween(),
                Motion::default(),
                start + Duration::from_millis(500),
            ),
        )
    });
    assert!(mid.1 && (0.5..1.5).contains(&mid.0), "{mid:?}");
    assert_eq!(settled, (2.0, false));
}

#[gpui::test]
fn retarget_continues_from_the_current_position(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(0));
    state.update(cx, |s, cx| s.go_to(1, tween(), cx));
    state.update(cx, |s, cx| s.go_to(0, tween(), cx));
    let (at, running) =
        state.read_with(cx, |s, _| s.at(tween(), Motion::default(), Instant::now()));
    assert!((0.0..0.1).contains(&at), "{at}");
    assert!(running);
}

#[gpui::test]
fn same_slot_is_idle(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(1));
    state.update(cx, |s, cx| s.go_to(1, tween(), cx));
    let at = state.read_with(cx, |s, _| s.at(tween(), Motion::default(), Instant::now()));
    assert_eq!(at, (1.0, false));
}
