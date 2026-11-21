use layout::PaneInstance;

use super::{ChatLocks, ShellPaneSource};

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
        crate::tab_looks::file_chat_of(instance).is_some_and(|chat| (self.locks)(chat))
    }
}

#[cfg(test)]
mod tests {
    use ideshell::{TabAccess, TabLooks as _};
    use protocol::PaneId;

    use super::*;
    use crate::browser_bars::BrowserBars;
    use crate::native_panes::NativePanes;
    use crate::new_tab_panes::NewTabPanes;
    use crate::serials::Serials;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;
    use crate::titles::Titles;

    fn pane(
        kind: &str,
        thread: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: protocol::PaneTypeId::new(kind).unwrap(),
            params: protocol::Params::default()
                .with("threadId", thread)
                .unwrap(),
            state: protocol::PaneState::default(),
        }
    }

    fn source() -> ShellPaneSource {
        ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            BrowserBars::new(
                crate::browser_ids::BrowserIds::default(),
                async_channel::unbounded().0,
            ),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        )
        .locking(std::rc::Rc::new(|chat| chat == "locked"))
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
                source.tab_look(&pane(kind, chat)).access,
                expected,
                "{name}"
            );
        }
    }
}
