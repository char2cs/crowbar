use std::time::Duration;

use gpui::{AppContext as _, TestAppContext};

use super::*;
use crate::cubic_bezier::CubicBezier;

const SPAN: Duration = Duration::from_millis(150);

fn tween() -> Tween {
    Tween::new(SPAN, CubicBezier::LINEAR)
}

fn position(state: &ToggleState, after: Duration, start: Instant) -> Position {
    state.position(tween(), Motion::default(), start + after)
}

#[gpui::test]
fn starting_value_does_not_animate(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(Pose::On));
    let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
    assert_eq!(at, Position::settled(1.0));
}

#[gpui::test]
fn flipping_eases_between_the_ends(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(Pose::Off));
    state.update(cx, |s, cx| s.set(Pose::On, cx));
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
    assert_eq!(late, Position::settled(1.0));
}

#[gpui::test]
fn flipping_back_eases_down(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(Pose::On));
    state.update(cx, |s, cx| s.set(Pose::Off, cx));
    let start = Instant::now();
    let mid = state.read_with(cx, |s, _| position(s, Duration::from_millis(75), start));
    assert!(mid.running && (mid.value - 0.5).abs() < 0.15, "{mid:?}");
}

#[gpui::test]
fn reduced_motion_jumps_to_the_target(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(Pose::Off));
    state.update(cx, |s, cx| s.set(Pose::On, cx));
    let later = Instant::now() + Duration::from_millis(1);
    let at = state.read_with(cx, |s, _| s.position(tween(), Motion::reduced(), later));
    assert_eq!(at, Position::settled(1.0));
}

#[gpui::test]
fn setting_the_same_value_keeps_it_idle(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::starting(Pose::On));
    state.update(cx, |s, cx| s.set(Pose::On, cx));
    let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
    assert!(!at.running);
}

#[gpui::test]
fn entrance_waits_then_eases(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::default());
    state.update(cx, |s, cx| s.set(Pose::On, cx));
    let start = Instant::now();
    let delayed = tween().delayed(SPAN);
    let at = |after_ms: u64| {
        state.read_with(cx, |s, _| {
            s.entered(
                delayed,
                Motion::default(),
                start + Duration::from_millis(after_ms),
            )
        })
    };
    assert_eq!(
        at(100),
        Position {
            value: 0.0,
            running: true
        }
    );
    let mid = at(225);
    assert!(mid.running && (mid.value - 0.5).abs() < 0.1, "{mid:?}");
    assert_eq!(at(400), Position::settled(1.0));
}

#[gpui::test]
fn entrance_is_zero_while_off(cx: &mut TestAppContext) {
    let state = cx.new(|_| ToggleState::default());
    let at = state.read_with(cx, |s, _| {
        s.entered(tween(), Motion::default(), Instant::now())
    });
    assert_eq!(at, Position::settled(0.0));
}
