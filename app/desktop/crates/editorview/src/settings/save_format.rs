#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum SaveFormat {
    #[default]
    AsTyped,
    Formatted,
}
