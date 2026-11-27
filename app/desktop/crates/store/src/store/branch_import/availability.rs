#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Availability {
    Importable,
    Imported,
    Protected,
}
