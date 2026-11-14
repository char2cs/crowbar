use gpui::TestAppContext;

use super::*;

const SPAN: Duration = Duration::from_millis(150);

fn position(state: &ToggleState, after: Duration, start: Instant) -> Position {
    state.position(SPAN, CubicBezier::LINEAR, Motion::default(), start + after)
}

#[gpui::test]
fn starting_value_does_not_animate(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(true));
    let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
    assert_eq!(at, Position { value: 1.0, running: false });
}

#[gpui::test]
fn flipping_eases_between_the_ends(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(false));
    state.update(cx, |s, cx| s.set(true, cx));
    let start = Instant::now();
    let (early, mid, late) = state.read_with(cx, |s, _| {
        (
            position(s, Duration::ZERO, start),
            position(s, Duration::from_millis(75), start),
            position(s, Duration::from_millis(300), start),
        )
    });
    assert!(early.running && early.value < 0.2, "{early:?}");
    assert!(mid.running && (mid.value - 0.5).abs() < 0.15, "{mid:?}");
    assert_eq!(late, Position { value: 1.0, running: false });
}

#[gpui::test]
fn reduced_motion_jumps_to_the_target(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(false));
    state.update(cx, |s, cx| s.set(true, cx));
    let later = Instant::now() + Duration::from_millis(1);
    let at = state.read_with(cx, |s, _| s.position(SPAN, CubicBezier::LINEAR, Motion::reduced(), later));
    assert_eq!(at, Position { value: 1.0, running: false });
}

#[gpui::test]
fn setting_the_same_value_keeps_it_idle(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(true));
    state.update(cx, |s, cx| s.set(true, cx));
    let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
    assert!(!at.running);
}

#[gpui::test]
fn entrance_waits_then_eases(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::default());
    state.update(cx, |s, cx| s.set(true, cx));
    let start = Instant::now();
    let at = |after_ms: u64| {
        state.read_with(cx, |s, _| {
            s.entered(SPAN, SPAN, CubicBezier::LINEAR, Motion::default(), start + Duration::from_millis(after_ms))
        })
    };
    assert_eq!(at(100), Position { value: 0.0, running: true });
    let mid = at(225);
    assert!(mid.running && (mid.value - 0.5).abs() < 0.1, "{mid:?}");
    assert_eq!(at(400), Position { value: 1.0, running: false });
}

#[gpui::test]
fn entrance_is_zero_while_off(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::default());
    let at = state.read_with(cx, |s, _| s.entered(SPAN, SPAN, CubicBezier::LINEAR, Motion::default(), Instant::now()));
    assert_eq!(at, Position { value: 0.0, running: false });
}
