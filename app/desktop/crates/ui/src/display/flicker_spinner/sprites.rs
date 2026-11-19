//! The flip-dot sprites the panes bundle; each spinner bakes one, picked at random.

use std::collections::hash_map::RandomState;
use std::hash::BuildHasher as _;

use crate::display::flicker_spinner::sprite::Sprite;

const SOURCES: &[&str] = &[
    include_str!("../../../../../../panes/src/components/ui/spinners/bar-chart.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/bounce.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/busy.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/classic-loading.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/comet.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/dot-dot-dot.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/flash.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/framer-loading.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/galaxy.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/gradient.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/importing.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/infinite.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/initialising.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/loading-gifs.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/loading-graph.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/loading.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/newton.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/pacman.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/ripple.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/rocket.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/rolling-dice.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/saving.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/searching.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/sleepy.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/syncing.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/target.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/the-claw.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/the-great-wave.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/tiny-spinner.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/tumbleweed.svg"),
    include_str!("../../../../../../panes/src/components/ui/spinners/unboxing.svg"),
];

pub(crate) fn pick() -> Option<Sprite> {
    let count = u64::try_from(SOURCES.len())
        .ok()
        .filter(|count| *count > 0)?;
    let roll = RandomState::new().hash_one(SOURCES.as_ptr()) % count;
    Sprite::parse(SOURCES.get(usize::try_from(roll).ok()?)?)
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;

    const STEP: Duration = Duration::from_millis(90);

    #[test]
    fn every_bundled_sprite_parses_and_steps_every_ninety_milliseconds() {
        for (index, source) in SOURCES.iter().enumerate() {
            let sprite = Sprite::parse(source).expect("bakes");
            assert_eq!(sprite.frame_at(Duration::ZERO).1, STEP, "sprite {index}");
        }
    }

    #[test]
    fn a_pick_bakes_one_of_the_sprites() {
        assert!(pick().is_some(), "a sprite");
    }
}
