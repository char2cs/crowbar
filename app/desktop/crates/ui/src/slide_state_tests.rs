use gpui::TestAppContext;

use super::*;

const SPAN: Duration = Duration::from_millis(200);

#[gpui::test]
fn slides_between_slots_and_settles(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(0));
    state.update(cx, |s, cx| s.go_to(2, SPAN, CubicBezier::LINEAR, Motion::default(), cx));
    let start = Instant::now();
    let (mid, settled) = state.read_with(cx, |s, _| {
        (
            s.at(SPAN, CubicBezier::LINEAR, Motion::default(), start + Duration::from_millis(100)),
            s.at(SPAN, CubicBezier::LINEAR, Motion::default(), start + Duration::from_millis(500)),
        )
    });
    assert!(mid.1 && mid.0 > 0.5 && mid.0 < 1.5, "{mid:?}");
    assert_eq!(settled, (2.0, false));
}

#[gpui::test]
fn retarget_continues_from_the_current_position(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(0));
    state.update(cx, |s, cx| s.go_to(1, SPAN, CubicBezier::LINEAR, Motion::default(), cx));
    state.update(cx, |s, cx| s.go_to(0, SPAN, CubicBezier::LINEAR, Motion::default(), cx));
    let (at, running) = state.read_with(cx, |s, _| s.at(SPAN, CubicBezier::LINEAR, Motion::default(), Instant::now()));
    assert!(at >= 0.0 && at < 0.1, "{at}");
    assert!(running);
}

#[gpui::test]
fn same_slot_is_idle(cx: &mut TestAppContext) {
    let state = cx.new(|_| SlideState::starting(1));
    state.update(cx, |s, cx| s.go_to(1, SPAN, CubicBezier::LINEAR, Motion::default(), cx));
    assert_eq!(state.read_with(cx, |s, _| s.at(SPAN, CubicBezier::LINEAR, Motion::default(), Instant::now())), (1.0, false));
}
