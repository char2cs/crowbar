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

    #[test]
    fn leaving_a_spot_forgets_it_only_while_the_pointer_is_still_over_it() {
        let mut state = FileDragState::pressed(FileMove::new("a.rs", FileKind::File), (0.0, 0.0));
        assert!(state.hover(folder("lib")), "reached lib");
        assert!(!state.hover(folder("lib")), "still on lib");
        assert!(state.hover(DropSpot::Root), "reached the panel");
        assert!(!state.leave(&folder("lib")), "lib was already left");
        assert_eq!(state.over, Some(DropSpot::Root));
        assert!(state.leave(&DropSpot::Root), "left the panel");
        assert_eq!(state.over, None);
    }
}
