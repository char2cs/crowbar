//! What the workspace decides about the Files menu.

use crate::lock::Lock;
use crate::shell_answers::ShellAnswers;

/// The workspace facts a Files menu is built from.
#[derive(Debug, Clone, Default)]
pub struct MenuScope {
    /// The workspace root, which absolute paths are built on.
    pub root: Option<String>,
    /// Whether the workspace's branch is locked.
    pub lock: Lock,
    /// Whether the workspace is on this computer, which offers Reveal in Finder.
    pub local: bool,
    /// Whether the workspace can generate an image.
    pub can_generate_image: bool,
    /// The shell's answers about which entries it serves.
    pub answers: ShellAnswers,
}

impl MenuScope {
    /// Whether the workspace's branch is locked.
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
