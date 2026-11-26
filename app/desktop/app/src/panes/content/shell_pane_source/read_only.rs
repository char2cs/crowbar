use layout::PaneInstance;

use super::{ChatLocks, ShellPaneSource};
use crate::panes::ide::tab_looks;

impl ShellPaneSource {
    pub(crate) fn locking(
        mut self,
        locks: ChatLocks,
    ) -> Self {
        self.locks = locks;
        self
    }

    pub(super) fn locked(
        &self,
        instance: &PaneInstance,
    ) -> bool {
        tab_looks::file_chat_of(instance).is_some_and(|chat| (self.locks)(chat))
    }
}

#[cfg(test)]
mod tests {
    use ideshell::{TabAccess, TabLooks as _};

    use super::*;
    use crate::panes::content::shell_pane_source::tests::idle_source;
    use crate::panes::tracking::titles::Titles;
    use crate::test_support::chat_pane;

    fn source() -> ShellPaneSource {
        idle_source(Titles::default()).locking(std::rc::Rc::new(|chat| chat == "locked"))
    }

    #[test]
    fn only_a_file_of_a_locked_chat_is_read_only() {
        let source = source();
        let cases = [
            (
                "a file of the locked chat",
                "core.editor",
                "locked",
                TabAccess::ReadOnly,
            ),
            (
                "a file of another chat",
                "core.editor",
                "open",
                TabAccess::Writable,
            ),
            (
                "a terminal of the locked chat",
                "core.terminal",
                "locked",
                TabAccess::Writable,
            ),
        ];
        for (name, kind, chat, expected) in cases {
            assert_eq!(
                source.tab_look(&chat_pane(("p1", kind), chat)).access,
                expected,
                "{name}"
            );
        }
    }
}
