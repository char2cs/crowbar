use layout::PaneInstance;

use super::{ChatNames, ShellPaneSource};

impl ShellPaneSource {
    pub(crate) fn naming(
        mut self,
        names: ChatNames,
    ) -> Self {
        self.names = names;
        self
    }

    pub(super) fn chat_name(
        &self,
        instance: &PaneInstance,
    ) -> Option<String> {
        crate::tab_looks::chat_of(instance).and_then(|chat| (self.names)(chat))
    }
}

#[cfg(test)]
mod tests {
    use ideshell::TabLooks as _;
    use protocol::{PaneId, PaneTitle};

    use super::*;
    use crate::browser_bars::BrowserBars;
    use crate::native_panes::NativePanes;
    use crate::new_tab_panes::NewTabPanes;
    use crate::serials::Serials;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;
    use crate::titles::Titles;

    fn chat(thread: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(thread).unwrap(),
            type_id: protocol::PaneTypeId::new("core.chat").unwrap(),
            params: protocol::Params::default()
                .with("threadId", thread)
                .unwrap(),
            state: protocol::PaneState::default(),
        }
    }

    fn source(titles: Titles) -> ShellPaneSource {
        ShellPaneSource::new(
            titles,
            Serials::above(&layout::Layout::default()),
            BrowserBars::new(
                crate::browser_ids::BrowserIds::default(),
                async_channel::unbounded().0,
            ),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
                crate::test_support::idle_editors(),
            ),
        )
        .naming(std::rc::Rc::new(|chat| {
            (chat == "held").then(|| "Perf fixture".to_owned())
        }))
    }

    #[test]
    fn a_chat_tab_reads_its_chat_s_name_before_its_page_has_ever_loaded() {
        let titles = Titles::default();
        titles.set(
            PaneId::new("unheld").unwrap(),
            PaneTitle::new("From the page").unwrap(),
        );
        let source = source(titles);
        let cases = [
            ("a chat the store holds", "held", "Perf fixture"),
            ("a chat it does not hold", "unheld", "From the page"),
        ];
        for (name, thread, label) in cases {
            let look = source.tab_look(&chat(thread));
            assert_eq!(look.label.as_ref(), label, "{name}");
        }
    }
}
