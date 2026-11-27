use crate::keystroke::modifiers::Modifiers;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PhysicalKey {
    pub code: String,
    pub modifiers: Modifiers,
}
