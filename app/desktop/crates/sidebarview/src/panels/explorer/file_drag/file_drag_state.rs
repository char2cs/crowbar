//! What the Files panel remembers about a row drag between pointer events.

use gpui::Task;
use sidebar::{DragArm, DropSpot, FileDrop, FileMove};

#[derive(Default)]
pub(crate) struct FileDragState {
    pub(crate) press: Option<(FileMove, DragArm)>,
    pub(crate) lifted: Option<FileMove>,
    pub(crate) over: Option<DropSpot>,
    pub(crate) pull: f32,
    pub(crate) unfolding: Option<Task<gpui::Result<()>>>,
}

impl FileDragState {
    pub(crate) fn pressed(
        row: FileMove,
        at: (f32, f32),
    ) -> Self {
        Self {
            press: Some((row, DragArm::file_at(at))),
            ..Self::default()
        }
    }

    pub(crate) fn lift(
        &mut self,
        at: (f32, f32),
    ) -> bool {
        if self.lifted.is_some() {
            return true;
        }
        let Some((row, arm)) = &self.press else {
            return false;
        };
        if !arm.started(at) {
            return false;
        }
        self.lifted = Some(row.clone());
        true
    }

    pub(crate) fn hover(
        &mut self,
        spot: DropSpot,
    ) -> bool {
        let changed = self.over.as_ref() != Some(&spot);
        self.over = Some(spot);
        changed
    }

    pub(crate) fn leave(
        &mut self,
        spot: &DropSpot,
    ) -> bool {
        let left = self.over.as_ref() == Some(spot);
        if left {
            self.over = None;
        }
        left
    }

    pub(crate) fn target(&self) -> Option<FileDrop> {
        let lifted = self.lifted.as_ref()?;
        lifted.onto(self.over.clone()?)
    }

    pub(crate) fn is_live(&self) -> bool {
        self.press.is_some() || self.lifted.is_some()
    }

    pub(crate) fn end(&mut self) {
        *self = Self::default();
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Expansion, FileKind};

    use super::*;

    fn folder(path: &str) -> DropSpot {
        DropSpot::Row {
            path: path.into(),
            kind: FileKind::Directory(Expansion::Collapsed),
        }
    }

    fn src() -> FileMove {
        FileMove::new("src", FileKind::Directory(Expansion::Expanded))
    }

    #[test]
    fn a_press_lifts_its_row_once_the_pointer_goes_five_pixels() {
        let mut state = FileDragState::pressed(src(), (10.0, 10.0));
        assert!(!state.lift((13.0, 13.0)), "under five pixels");
        assert!(state.lift((15.0, 10.0)), "five pixels");
        assert_eq!(state.lifted, Some(src()));
        assert!(
            !FileDragState::default().lift((90.0, 90.0)),
            "nothing pressed"
        );
    }

    #[test]
    fn leaving_a_spot_forgets_it_only_while_the_pointer_is_still_over_it() {
        let mut state = FileDragState::pressed(src(), (0.0, 0.0));
        assert!(state.hover(folder("lib")), "reached lib");
        assert!(!state.hover(folder("lib")), "still on lib");
        assert!(state.hover(DropSpot::Root), "reached the panel");
        assert!(!state.leave(&folder("lib")), "lib was already left");
        assert_eq!(state.over, Some(DropSpot::Root));
        assert!(state.leave(&DropSpot::Root), "left the panel");
        assert_eq!(state.over, None);
    }

    #[test]
    fn the_target_is_the_spot_under_the_lifted_row_unless_it_refuses_it() {
        let cases = [
            (Some(folder("lib")), true),
            (Some(folder("src/inner")), false),
            (None, false),
        ];
        for (over, accepts) in cases {
            let mut state = FileDragState::pressed(src(), (0.0, 0.0));
            state.over = over.clone();
            assert_eq!(state.target(), None, "not lifted yet: {over:?}");
            state.lift((0.0, 9.0));
            assert_eq!(state.target().is_some(), accepts, "{over:?}");
            state.end();
            assert!(!state.is_live(), "ended");
        }
    }
}
