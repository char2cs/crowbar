use super::*;

fn migrating(step: Option<MigrationStep>, fraction: Option<f32>) -> StartupScreen {
    StartupScreen::Migrating { step, fraction }
}

#[test]
fn only_ready_shows_the_layout() {
    let screens = [
        StartupScreen::Starting,
        migrating(None, None),
        StartupScreen::Halted {
            reason: HaltReason::StepFailed,
            message: "boom".to_owned(),
        },
        StartupScreen::Unavailable(Outage::Stopped),
        StartupScreen::Ready,
    ];
    let shown: Vec<bool> = screens
        .iter()
        .map(|screen| screen.phase().shows_layout())
        .collect();
    assert_eq!(shown, [false, false, false, false, true]);
}

#[test]
fn migrating_phase_carries_the_step_and_progress() {
    let step = MigrationStep::new("Rebuild index", 2, 5);
    let phase = migrating(Some(step), Some(0.4)).phase();
    assert_eq!(
        phase,
        AppPhase::Migrating {
            state: "Step 2 of 5: Rebuild index".to_owned(),
            progress: Some(0.4)
        }
    );
}

#[test]
fn announcements_name_the_state_step_and_percent() {
    let step = MigrationStep::new("Rebuild index", 2, 5);
    assert_eq!(
        migrating(Some(step), Some(0.4)).announcement(),
        "Upgrading your data. Step 2 of 5: Rebuild index. 40 percent."
    );
    assert_eq!(migrating(None, None).announcement(), "Upgrading your data.");
    let halted = StartupScreen::Halted {
        reason: HaltReason::LayoutNewer,
        message: "layout 9 is newer than 7".to_owned(),
    };
    assert!(
        halted
            .announcement()
            .starts_with("Crowbar could not start. ")
    );
    assert!(halted.announcement().ends_with("layout 9 is newer than 7"));
}

#[test]
fn waiting_and_failure_are_exclusive() {
    let screens = [
        (StartupScreen::Starting, true, false),
        (migrating(None, None), true, false),
        (
            StartupScreen::Unavailable(Outage::Unresponsive),
            false,
            true,
        ),
        (StartupScreen::Ready, false, false),
    ];
    for (screen, waiting, failure) in screens {
        assert_eq!(screen.is_waiting(), waiting, "{screen:?}");
        assert_eq!(screen.is_failure(), failure, "{screen:?}");
    }
}

#[test]
fn percent_rounds_and_clamps() {
    const CASES: &[(f32, u8)] = &[(0.0, 0), (0.404, 40), (0.996, 100), (7.0, 100), (-1.0, 0)];
    for &(fraction, expected) in CASES {
        assert_eq!(percent(fraction), expected, "{fraction}");
    }
}
