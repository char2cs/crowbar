use std::rc::Rc;

use gpui::{TestAppContext, px};

use super::*;
use crate::startup_command::StartupCommand;
use crate::test_support::{RecordingActions, click_across, measure, open};

fn gave_up() -> Outage {
    Outage::GaveUp {
        report: "crowbar 1.0\nexit status 1".to_owned(),
    }
}

fn sink() -> (Rc<RecordingActions>, Rc<dyn StartupActions>) {
    let recording = Rc::new(RecordingActions::default());
    let shared = Rc::clone(&recording) as Rc<dyn StartupActions>;
    (recording, shared)
}

#[gpui::test]
fn every_outage_renders_a_card_of_the_fixed_width(cx: &mut TestAppContext) {
    let outages = [
        Outage::Unresponsive,
        Outage::Recovering {
            attempt: 2,
            reason: "exit status 1".to_owned(),
        },
        gave_up(),
        Outage::Stopped,
    ];
    for outage in outages {
        let (_, actions) = sink();
        let bounds = measure(cx, move |_, _| OutagePanel::new(&outage, &actions));
        assert_eq!(f32::from(bounds.size.width), WIDTH);
    }
}

#[gpui::test]
fn the_report_of_a_supervisor_that_gave_up_can_be_copied(cx: &mut TestAppContext) {
    let (_, actions) = sink();
    let with_report = measure(cx, {
        let actions = Rc::clone(&actions);
        move |_, _| OutagePanel::new(&gave_up(), &actions)
    });
    let without = measure(cx, move |_, _| OutagePanel::new(&Outage::Stopped, &actions));
    assert!(with_report.size.height > without.size.height + px(40.0));
}

#[gpui::test]
fn a_stopped_daemon_offers_every_command(cx: &mut TestAppContext) {
    let (recording, actions) = sink();
    let bounds = measure(cx, {
        let actions = Rc::clone(&actions);
        move |_, _| OutagePanel::new(&Outage::Stopped, &actions)
    });
    let cx = open(cx, move |_, _| OutagePanel::new(&Outage::Stopped, &actions));
    click_across(cx, bounds);
    let performed = recording.performed();
    for command in [
        StartupCommand::Retry,
        StartupCommand::OpenLog,
        StartupCommand::Quit,
    ] {
        assert!(performed.contains(&command), "{command:?} in {performed:?}");
    }
}

#[gpui::test]
fn a_restarting_daemon_offers_only_the_log(cx: &mut TestAppContext) {
    let (recording, actions) = sink();
    let outage = Outage::Unresponsive;
    let bounds = measure(cx, {
        let (actions, outage) = (Rc::clone(&actions), outage.clone());
        move |_, _| OutagePanel::new(&outage, &actions)
    });
    let cx = open(cx, move |_, _| OutagePanel::new(&outage, &actions));
    click_across(cx, bounds);
    let performed = recording.performed();
    assert!(performed.contains(&StartupCommand::OpenLog));
    assert!(
        performed
            .iter()
            .all(|command| *command == StartupCommand::OpenLog)
    );
}
