//! The selection and the composition range of a field.

use std::ops::Range;

pub(crate) trait FieldSelection {
    fn marked_range(&self) -> Option<Range<usize>>;
    fn selected_range(&self) -> Range<usize>;
}
