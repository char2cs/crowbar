use layout::{Layout, PaneGroup};
use protocol::PaneId;

pub(crate) fn focused_chat(
    layout: &Layout,
    space: &str,
) -> Option<String> {
    let tab = layout.showing(space)?;
    chat_of_pane(layout, tab.focused()?)
}

pub(crate) fn chat_of_pane(
    layout: &Layout,
    pane: &PaneId,
) -> Option<String> {
    let tab = layout.tab_of(pane)?;
    let chat = tab.group_of(pane).and_then(PaneGroup::chat);
    let instance = layout.pane(chat.unwrap_or(pane))?;
    instance.params.get("threadId").map(str::to_owned)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panes::kinds::core_panes::CHAT;
    use crate::test_support::one_tab;

    #[test]
    fn the_focused_pane_names_its_groups_chat() {
        let mut rig = crate::test_support::shell_rig();
        assert_eq!(focused_chat(rig.shell.layout(), ""), None, "nothing open");
        crate::test_support::open_chat(&mut rig.shell, "c7");
        assert_eq!(focused_chat(rig.shell.layout(), "").as_deref(), Some("c7"));
        let browser = crate::panes::kinds::pane_spec::PaneSpec::browser("b1").unwrap();
        rig.shell.open_spec(browser);
        let layout = rig.shell.layout();
        let focused = layout.showing("").and_then(layout::Tab::focused);
        let browser_focused = focused
            .and_then(|pane| layout.pane(pane))
            .is_some_and(|pane| pane.params.get("browserId") == Some("b1"));
        assert!(browser_focused, "the browser takes the focus");
        assert_eq!(
            focused_chat(layout, "").as_deref(),
            Some("c7"),
            "a browser beside it"
        );
    }

    #[test]
    fn a_pane_without_a_chat_names_none() {
        let layout = one_tab([crate::test_support::pane("p1", CHAT)]);
        assert_eq!(focused_chat(&layout, ""), None);
    }
}
