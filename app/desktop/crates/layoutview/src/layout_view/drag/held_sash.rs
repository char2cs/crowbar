use layout::Command;

use crate::layout_view::drag::chat_sash_grab::ChatSashGrab;
use crate::layout_view::drag::sash_grab::SashGrab;

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum HeldSash {
    Split(SashGrab),
    Chat(ChatSashGrab),
}

impl HeldSash {
    pub(crate) fn resize(
        &self,
        pointer: (f32, f32),
    ) -> Command {
        match self {
            Self::Split(grab) => grab.resize(pointer),
            Self::Chat(grab) => grab.resize(pointer),
        }
    }
}

#[cfg(test)]
mod tests {
    use layout::{Presentation, Rect, SidebarSide};

    use super::*;
    use crate::test_support::{layout_of, open_tab, pane_id};

    #[test]
    fn a_held_chat_sash_asks_for_a_chat_resize() {
        let mut layout = layout_of("a", &[]);
        open_tab(&mut layout, "t1");
        let group = layout
            .showing("")
            .and_then(|tab| tab.group_of(&pane_id("a")))
            .expect("group");
        let area = Rect {
            x: 0.0,
            y: 0.0,
            width: 1006.0,
            height: 800.0,
        };
        let shown = Presentation::of(group, area, SidebarSide::Left);
        let grab = ChatSashGrab::begin(pane_id("a"), (&shown, area), SidebarSide::Left, (0.0, 0.0));
        let held = HeldSash::Chat(grab.expect("grab"));
        assert!(matches!(
            held.resize((9.0, 0.0)),
            Command::ResizeChat { .. }
        ));
    }
}
