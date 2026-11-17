//! Whether a field takes input.

pub(crate) trait FieldEditable {
    fn editable(&self) -> bool;
}
