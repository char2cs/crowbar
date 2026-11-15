//! Where each tree row was last laid out, in window coordinates.

use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use gpui::{Bounds, Pixels};

use crate::row_id::RowId;

type Layout = HashMap<RowId, Bounds<Pixels>>;

#[derive(Debug, Clone, Default)]
pub(crate) struct RowBounds(Rc<RefCell<Layout>>);

impl RowBounds {
    pub(crate) fn record(
        &self,
        row: &RowId,
        bounds: Bounds<Pixels>,
    ) {
        self.0.borrow_mut().insert(row.clone(), bounds);
    }

    pub(crate) fn of(
        &self,
        row: &RowId,
    ) -> Option<Bounds<Pixels>> {
        self.0.borrow().get(row).copied()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px, size};

    use super::*;

    #[test]
    fn the_latest_layout_of_a_row_wins_and_clones_share_it() {
        let row = RowId::from("r");
        let bounds = RowBounds::default();
        let shared = bounds.clone();
        assert_eq!(bounds.of(&row), None, "unknown");
        let first = Bounds::new(point(px(0.0), px(0.0)), size(px(10.0), px(10.0)));
        let second = Bounds::new(point(px(0.0), px(5.0)), size(px(10.0), px(10.0)));
        bounds.record(&row, first);
        shared.record(&row, second);
        assert_eq!(bounds.of(&row), Some(second));
    }
}
