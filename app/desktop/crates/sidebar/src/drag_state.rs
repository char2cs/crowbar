//! What the sidebar remembers about a row drag between pointer events.

use crate::drag_press::DragPress;
use crate::drop_hint::DropHint;
use crate::drop_mode::DropMode;
use crate::line_slide::LineSlide;
use crate::row_id::RowId;

#[derive(Debug, Default)]
pub(crate) struct DragState {
    pub(crate) press: Option<DragPress>,
    pub(crate) lifted: Option<RowId>,
    pub(crate) hint: Option<DropHint>,
    pub(crate) pull: f32,
    pub(crate) slide: Option<LineSlide>,
}

impl DragState {
    pub(crate) fn end(&mut self) {
        *self = Self::default();
    }

    pub(crate) fn mode_on(
        &self,
        row: &RowId,
    ) -> Option<DropMode> {
        self.hint
            .as_ref()
            .filter(|hint| &hint.row == row)
            .map(|hint| hint.mode)
    }

    pub(crate) fn is_lifted(
        &self,
        row: &RowId,
    ) -> bool {
        self.lifted.as_ref() == Some(row)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ending_a_drag_forgets_everything() {
        let row = RowId::from("r");
        let hint = DropHint {
            row: row.clone(),
            mode: DropMode::Into,
        };
        let mut state = DragState {
            lifted: Some(row.clone()),
            hint: Some(hint),
            pull: 4.0,
            ..DragState::default()
        };
        assert_eq!(state.mode_on(&row), Some(DropMode::Into), "hint");
        assert!(state.is_lifted(&row), "lifted");
        state.end();
        assert_eq!(state.mode_on(&row), None, "hint cleared");
        assert!(!state.is_lifted(&row), "lift cleared");
        assert!(state.pull.abs() < f32::EPSILON, "pull cleared");
    }
}
