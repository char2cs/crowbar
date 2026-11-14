use std::cell::RefCell;
use std::rc::Rc;
use std::time::Duration;

use gpui::{Entity, TestAppContext, VisualTestContext};

use super::*;
use crate::migration_step::MigrationStep;
use crate::outage::Outage;
use crate::startup_command::StartupCommand;
use crate::test_support::{ManualClock, RecordingActions, click_across};

const REVEAL: Duration = Duration::from_millis(250);
const VISIBLE: Duration = Duration::from_millis(600);

struct Harness {
    view: Entity<StartupView>,
    clock: Rc<ManualClock>,
    actions: Rc<RecordingActions>,
    events: Rc<RefCell<Vec<StartupEvent>>>,
}

fn at(millis: u64) -> Duration {
    Duration::from_millis(millis)
}

fn open(cx: &mut TestAppContext, timing: StartupTiming) -> (Harness, &mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).expect("the design system installs"));
    let clock = Rc::new(ManualClock::default());
    let actions = Rc::new(RecordingActions::default());
    let (view_clock, view_actions) = (Rc::clone(&clock), Rc::clone(&actions));
    let (view, cx) =
        cx.add_window_view(move |_, cx| StartupView::new(timing, view_clock, view_actions, cx));
    let events = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&events);
    cx.update(|_, cx| {
        cx.subscribe(&view, move |_, event: &StartupEvent, _| {
            sink.borrow_mut().push(*event);
        })
        .detach();
    });
    let harness = Harness {
        view,
        clock,
        actions,
        events,
    };
    (harness, cx)
}

fn feed(harness: &Harness, cx: &mut VisualTestContext, input: StartupInput) {
    harness.view.update(cx, |view, cx| view.apply(input, cx));
}

fn pass(harness: &Harness, cx: &mut VisualTestContext, to: Duration) {
    let elapsed = to.saturating_sub(harness.clock.now());
    harness.clock.set(to);
    cx.executor().advance_clock(elapsed);
    cx.run_until_parked();
}

fn instant_timing() -> StartupTiming {
    StartupTiming::new(Duration::ZERO, Duration::ZERO)
}

#[gpui::test]
fn a_start_that_finishes_inside_the_blank_time_hands_over_without_a_fade(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, StartupTiming::new(REVEAL, VISIBLE));
    harness.clock.set(at(100));
    feed(&harness, cx, StartupInput::Ready);
    assert!(harness.view.read_with(cx, |view, _| view.is_handed_over()));
    assert_eq!(*harness.events.borrow(), [StartupEvent::HandedOver]);
}

#[gpui::test]
fn a_visible_start_waits_out_the_minimum_then_fades_and_hands_over(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, StartupTiming::new(REVEAL, VISIBLE));
    pass(&harness, cx, at(300));
    feed(&harness, cx, StartupInput::Ready);
    assert_eq!(
        harness.view.read_with(cx, |view, _| view.phase()),
        AppPhase::Starting
    );
    pass(&harness, cx, REVEAL + VISIBLE);
    assert_eq!(
        harness.view.read_with(cx, |view, _| view.phase()),
        AppPhase::Ready
    );
    assert!(!harness.view.read_with(cx, |view, _| view.is_handed_over()));
    pass(&harness, cx, REVEAL + VISIBLE + timing::SLOW);
    assert!(harness.view.read_with(cx, |view, _| view.is_handed_over()));
    assert_eq!(*harness.events.borrow(), [StartupEvent::HandedOver]);
}

#[gpui::test]
fn inputs_after_the_hand_over_are_ignored(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, instant_timing());
    feed(&harness, cx, StartupInput::Ready);
    pass(&harness, cx, timing::SLOW);
    feed(&harness, cx, StartupInput::Unavailable(Outage::Stopped));
    assert_eq!(
        harness.view.read_with(cx, |view, _| view.phase()),
        AppPhase::Ready
    );
    assert_eq!(harness.events.borrow().len(), 1);
}

#[gpui::test]
fn migration_progress_reaches_the_protocol_phase_and_the_announcement(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, instant_timing());
    let step = MigrationStep::new("Rebuild index", 2, 5);
    feed(&harness, cx, StartupInput::Migrating { step: Some(step) });
    let (phase, spoken) = harness
        .view
        .read_with(cx, |view, _| (view.phase(), view.announcement()));
    assert_eq!(
        phase,
        AppPhase::Migrating {
            state: "Step 2 of 5: Rebuild index".to_owned(),
            progress: Some(0.4)
        }
    );
    assert_eq!(
        spoken,
        "Upgrading your data. Step 2 of 5: Rebuild index. 40 percent."
    );
}

fn click_everywhere(cx: &mut VisualTestContext) {
    cx.run_until_parked();
    let area = cx.update(|window, _| gpui::Bounds {
        origin: gpui::Point::default(),
        size: window.viewport_size(),
    });
    click_across(cx, area);
}

#[gpui::test]
fn the_halt_screen_can_quit(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, instant_timing());
    feed(
        &harness,
        cx,
        StartupInput::Halted {
            reason: Some("layout_newer".to_owned()),
            message: "layout 9 is newer than 7".to_owned(),
        },
    );
    click_everywhere(cx);
    let performed = harness.actions.performed();
    assert!(performed.contains(&StartupCommand::Quit), "{performed:?}");
    assert!(!performed.contains(&StartupCommand::Retry), "{performed:?}");
}

#[gpui::test]
fn a_crash_loop_can_retry(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, instant_timing());
    feed(
        &harness,
        cx,
        StartupInput::Unavailable(Outage::GaveUp {
            report: "log".to_owned(),
        }),
    );
    click_everywhere(cx);
    let performed = harness.actions.performed();
    assert!(performed.contains(&StartupCommand::Retry), "{performed:?}");
}
