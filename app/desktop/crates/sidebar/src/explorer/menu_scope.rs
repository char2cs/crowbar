pub(super) mod shell_answers;

use crate::explorer::menu_scope::shell_answers::ShellAnswers;
use crate::shell::Lock;

#[derive(Debug, Clone, Default)]
pub struct MenuScope {
    pub root: Option<String>,
    pub lock: Lock,
    pub local: bool,
    pub can_generate_image: bool,
    pub answers: ShellAnswers,
}

impl MenuScope {
    pub fn is_locked(&self) -> bool {
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
