use layout::Layout;
use protocol::{PaneId, ShellFrame};

use crate::error::Result;
use crate::pane_host::PaneHost;

fn chat_fills(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let group = layout.tab_of(pane).and_then(|tab| tab.group_of(pane));
    group.is_some_and(|group| {
        group.chat() == Some(pane) && group.tabs().is_empty() && !group.editor_open()
    })
}

impl PaneHost {
    pub(crate) fn send_groups(
        &mut self,
        layout: &Layout,
    ) -> Result<()> {
        let changed: Vec<(PaneId, bool)> = self
            .panes
            .iter_mut()
            .filter(|(_, runtime)| runtime.is_mounted() && runtime.type_id.is_chat())
            .filter_map(|(id, runtime)| {
                let fills = chat_fills(layout, id);
                let told = runtime.chat_fills.replace(fills);
                (told != Some(fills)).then(|| (id.clone(), fills))
            })
            .collect();
        for (pane, chat_fills) in changed {
            self.send(&pane, &ShellFrame::Group { chat_fills })?;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::*;
    use protocol::ShellFrame;

    fn groups(f: &Fixture) -> Vec<bool> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                ShellFrame::Group { chat_fills } => Some(chat_fills),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_chat_learns_whether_it_fills_its_group_on_mount_and_on_each_change_only() {
        let mut f = fixture();
        let chat = instance("p1", "core.chat");
        let tab = instance("t1", "core.term");
        f.sync(std::slice::from_ref(&chat), &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        assert_eq!(groups(&f), [true], "alone, on mount");
        f.sync(std::slice::from_ref(&chat), &["p1"], Some("p1"));
        assert_eq!(groups(&f), [true], "nothing changed, nothing sent");
        f.sync(&[chat, tab], &["p1", "t1"], Some("t1"));
        assert_eq!(groups(&f), [true, false], "an editor tab joined");
    }
}
