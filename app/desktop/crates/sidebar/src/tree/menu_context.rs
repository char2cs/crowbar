use crate::shell::MoveTarget;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct MenuContext {
    pub targets: Vec<MoveTarget>,
    pub running: Option<String>,
}
