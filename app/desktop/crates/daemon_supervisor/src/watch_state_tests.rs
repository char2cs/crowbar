use super::*;
use crate::probe::Probe;

#[test]
fn forgetting_clears_the_streak_and_the_silent_stretch() {
    let mut state = WatchState::new(2);
    state.tracker.observe(Probe::Unserved);
    state.silent_since = Some((Instant::now(), None));
    state.forget();
    assert!(state.silent_since.is_none());
    assert!(
        !state.tracker.observe(Probe::Unserved),
        "the streak restarted from zero"
    );
}
