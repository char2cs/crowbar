use layout::PaneInstance;

use super::{ChatNames, ShellPaneSource};
use crate::panes::ide::tab_looks;

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
        tab_looks::chat_of(instance).and_then(|chat| (self.names)(chat))
    }
}

#[cfg(test)]
mod tests {
    use ideshell::TabLooks as _;
    use protocol::{PaneId, PaneTitle};

    use super::*;
    use crate::panes::content::shell_pane_source::tests::idle_source;
    use crate::panes::tracking::titles::Titles;

    fn chat(thread: &str) -> PaneInstance {
        crate::test_support::chat_pane((thread, "core.chat"), thread)
    }

    fn source(titles: Titles) -> ShellPaneSource {
        idle_source(titles).naming(std::rc::Rc::new(|chat| {
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
