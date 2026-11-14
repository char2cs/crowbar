use super::*;
use crate::outage::Outage;

const REVEAL: Duration = Duration::from_millis(250);
const VISIBLE: Duration = Duration::from_millis(600);

fn at(millis: u64) -> Duration {
    Duration::from_millis(millis)
}

fn machine() -> StartupMachine {
    StartupMachine::new(StartupTiming::new(REVEAL, VISIBLE), Duration::ZERO)
}

fn step(index: u32, total: u32) -> StartupInput {
    StartupInput::Migrating {
        step: Some(MigrationStep::new(format!("step {index}"), index, total)),
    }
}

fn fraction(machine: &StartupMachine) -> Option<f32> {
    match machine.screen() {
        StartupScreen::Migrating { fraction, .. } => *fraction,
        other => panic!("not migrating: {other:?}"),
    }
}

fn migrating_machine() -> StartupMachine {
    let mut machine = machine();
    machine.apply(step(1, 5), at(300));
    machine.tick(at(900));
    machine
}

#[test]
fn a_start_that_finishes_before_the_reveal_never_draws() {
    let mut machine = machine();
    machine.apply(StartupInput::Ready, at(100));
    assert_eq!(machine.screen(), &StartupScreen::Ready);
    assert!(!machine.is_revealed());
    assert_eq!(machine.deadline(), None);
}

#[test]
fn ready_after_the_reveal_waits_until_the_screen_was_visible_long_enough() {
    let mut machine = machine();
    machine.tick(at(300));
    machine.apply(StartupInput::Ready, at(300));
    assert_eq!(machine.screen(), &StartupScreen::Starting);
    assert_eq!(machine.deadline(), Some(REVEAL + VISIBLE));
    machine.tick(at(849));
    assert_eq!(machine.screen(), &StartupScreen::Starting);
    machine.tick(at(850));
    assert_eq!(machine.screen(), &StartupScreen::Ready);
    assert_eq!(machine.deadline(), None);
}

#[test]
fn a_long_visible_screen_is_replaced_at_once() {
    let mut machine = machine();
    machine.tick(at(5_000));
    machine.apply(StartupInput::Ready, at(5_000));
    assert_eq!(machine.screen(), &StartupScreen::Ready);
}

#[test]
fn the_deadline_names_the_reveal_before_it_happens() {
    let mut machine = machine();
    assert_eq!(machine.deadline(), Some(REVEAL));
    machine.tick(REVEAL);
    assert!(machine.is_revealed());
    assert_eq!(machine.deadline(), None);
}

#[test]
fn only_the_latest_queued_screen_survives() {
    let mut machine = machine();
    machine.tick(at(300));
    machine.apply(step(1, 5), at(400));
    machine.apply(StartupInput::Ready, at(500));
    assert_eq!(machine.screen(), &StartupScreen::Starting);
    machine.tick(at(850));
    assert_eq!(machine.screen(), &StartupScreen::Ready);
}

#[test]
fn failures_show_at_once_and_drop_the_queue() {
    let mut machine = machine();
    machine.tick(at(300));
    machine.apply(StartupInput::Ready, at(300));
    machine.apply(StartupInput::Unavailable(Outage::Stopped), at(310));
    assert_eq!(
        machine.screen(),
        &StartupScreen::Unavailable(Outage::Stopped)
    );
    assert_eq!(machine.deadline(), None);
}

#[test]
fn a_failure_inside_the_blank_time_is_drawn_immediately() {
    let mut machine = machine();
    machine.apply(
        StartupInput::Halted {
            reason: Some("layout_newer".to_owned()),
            message: "newer".to_owned(),
        },
        at(50),
    );
    assert!(machine.is_revealed());
    assert_eq!(
        machine.screen(),
        &StartupScreen::Halted {
            reason: HaltReason::LayoutNewer,
            message: "newer".to_owned()
        }
    );
}

#[test]
fn leaving_a_failure_shows_the_next_screen_at_once() {
    let mut machine = machine();
    machine.apply(StartupInput::Unavailable(Outage::Stopped), at(2_000));
    machine.apply(StartupInput::Starting, at(2_010));
    assert_eq!(machine.screen(), &StartupScreen::Starting);
    assert!(machine.is_revealed());
}

#[test]
fn progress_never_goes_backwards_within_a_run() {
    let mut machine = migrating_machine();
    machine.apply(step(3, 5), at(1_000));
    machine.apply(step(2, 5), at(1_100));
    assert_eq!(fraction(&machine), Some(0.6));
    let StartupScreen::Migrating { step, .. } = machine.screen() else {
        panic!("not migrating");
    };
    assert_eq!(step.as_ref().map(MigrationStep::index), Some(3));
}

#[test]
fn a_report_without_a_step_keeps_the_last_known_one() {
    let mut machine = migrating_machine();
    machine.apply(StartupInput::Migrating { step: None }, at(1_000));
    assert_eq!(fraction(&machine), Some(0.2));
}

#[test]
fn a_new_run_starts_progress_over() {
    let mut machine = migrating_machine();
    machine.apply(step(4, 5), at(1_000));
    machine.apply(
        StartupInput::Unavailable(Outage::Recovering {
            attempt: 1,
            reason: "exit 1".to_owned(),
        }),
        at(1_100),
    );
    machine.apply(StartupInput::Starting, at(1_200));
    machine.apply(step(1, 5), at(1_300));
    machine.tick(at(2_000));
    assert_eq!(fraction(&machine), Some(0.2));
}

#[test]
fn step_updates_inside_migrating_are_not_held() {
    let mut machine = migrating_machine();
    machine.apply(step(2, 5), at(901));
    assert_eq!(fraction(&machine), Some(0.4));
}

#[test]
fn a_plan_without_steps_is_indeterminate() {
    let mut machine = machine();
    machine.apply(step(1, 0), at(10));
    assert_eq!(fraction(&machine), None);
}

#[test]
fn a_late_tick_cannot_move_time_backwards() {
    let mut machine = machine();
    machine.tick(at(1_000));
    machine.tick(at(10));
    assert!(machine.is_revealed());
}
