//! Whether a field takes input.

pub(crate) trait Editable {
    fn editable(&self) -> bool;
}
