//! What the workspace decides about the Files menu.

use crate::lock::Lock;
use crate::shell_answers::ShellAnswers;

#[derive(Debug, Clone, Default)]
pub(crate) struct MenuScope {
    pub(crate) root: Option<String>,
    pub(crate) lock: Lock,
    pub(crate) local: bool,
    pub(crate) can_generate_image: bool,
    pub(crate) answers: ShellAnswers,
}

impl MenuScope {
    pub(crate) fn is_locked(&self) -> bool {
        self.lock == Lock::Locked
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_locked_scope_is_locked() {
        let locked = MenuScope {
            lock: Lock::Locked,
            ..MenuScope::default()
        };
        assert!(locked.is_locked());
        assert!(!MenuScope::default().is_locked());
    }
}
