use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

fn width_of(cx: &mut TestAppContext, panel: impl Fn() -> WaitingPanel + 'static) -> f32 {
    f32::from(measure(cx, move |_, _| panel()).size.width)
}

#[gpui::test]
fn every_waiting_screen_is_as_wide_as_its_bar(cx: &mut TestAppContext) {
    let step = MigrationStep::new("Rebuild the search index", 2, 5);
    assert_eq!(width_of(cx, WaitingPanel::starting), WIDTH);
    assert_eq!(
        width_of(cx, move || WaitingPanel::migrating(
            Some(&step),
            Some(0.4),
            0.2
        )),
        WIDTH
    );
    assert_eq!(
        width_of(cx, || WaitingPanel::migrating(None, None, 0.0)),
        WIDTH
    );
}

#[gpui::test]
fn the_migration_screen_adds_the_footnote(cx: &mut TestAppContext) {
    let starting = measure(cx, |_, _| WaitingPanel::starting());
    let migrating = measure(cx, |_, _| WaitingPanel::migrating(None, None, 0.0));
    assert!(migrating.size.height > starting.size.height);
}
