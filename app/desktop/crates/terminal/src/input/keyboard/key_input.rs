use crate::input::modifiers::Modifiers;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct KeyInput {
    pub key: String,
    pub text: Option<String>,
    pub modifiers: Modifiers,
}
