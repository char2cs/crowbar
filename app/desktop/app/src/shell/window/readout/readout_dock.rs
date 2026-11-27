use sidebar::{DockSide, Preferences, Visibility};

#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub(crate) enum ReadoutDock {
    #[default]
    Top,
    Left(f32),
    Right(f32),
}

impl ReadoutDock {
    pub(crate) fn of(preferences: &Preferences) -> Self {
        match (preferences.visibility, preferences.side) {
            (Visibility::Closed, _) => Self::Top,
            (Visibility::Open, DockSide::Left) => Self::Left(preferences.width),
            (Visibility::Open, DockSide::Right) => Self::Right(preferences.width),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_open_sidebar_holds_the_readout_at_its_foot_and_a_closed_one_leaves_it_on_top() {
        let at = |visibility, side| Preferences {
            visibility,
            side,
            width: 300.0,
            ..Preferences::default()
        };
        assert_eq!(
            ReadoutDock::of(&at(Visibility::Open, DockSide::Left)),
            ReadoutDock::Left(300.0)
        );
        assert_eq!(
            ReadoutDock::of(&at(Visibility::Open, DockSide::Right)),
            ReadoutDock::Right(300.0)
        );
        assert_eq!(
            ReadoutDock::of(&at(Visibility::Closed, DockSide::Left)),
            ReadoutDock::Top
        );
    }
}
