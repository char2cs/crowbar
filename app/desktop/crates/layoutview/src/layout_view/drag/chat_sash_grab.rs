use layout::{Command, Presentation, Rect, SidebarSide, ViewMode};
use protocol::PaneId;

const WITH_POINTER: f32 = 1.0;
const AGAINST_POINTER: f32 = -1.0;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct ChatSashGrab {
    chat: PaneId,
    mode: ViewMode,
    area: Rect,
    extent: f32,
    pointer: f32,
    sign: f32,
}

impl ChatSashGrab {
    pub(crate) fn begin(
        chat: PaneId,
        laid: (&Presentation, Rect),
        side: SidebarSide,
        pointer: (f32, f32),
    ) -> Option<Self> {
        let (shown, area) = laid;
        let at = shown.chat?;
        let (extent, sign) = match (shown.mode, side) {
            (ViewMode::Stacked, _) => (at.height, WITH_POINTER),
            (_, SidebarSide::Right) => (at.width, AGAINST_POINTER),
            _ => (at.width, WITH_POINTER),
        };
        Some(Self {
            chat,
            mode: shown.mode,
            area,
            extent,
            pointer: along(shown.mode, pointer),
            sign,
        })
    }

    pub(crate) fn resize(
        &self,
        pointer: (f32, f32),
    ) -> Command {
        let moved = along(self.mode, pointer) - self.pointer;
        let wanted = self.extent + self.sign * moved;
        Command::ResizeChat {
            pane: self.chat.clone(),
            share: Presentation::chat_share_for(self.mode, self.area, wanted),
        }
    }
}

fn along(
    mode: ViewMode,
    (x, y): (f32, f32),
) -> f32 {
    match mode {
        ViewMode::Stacked => y,
        _ => x,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{layout_of, open_tab, pane_id};

    #[test]
    fn a_chat_docked_past_its_sash_grows_as_the_pointer_moves_back() {
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
        let share = |side: SidebarSide, moved: f32| {
            let shown = Presentation::of(group, area, side);
            let grab = ChatSashGrab::begin(pane_id("a"), (&shown, area), side, (500.0, 0.0));
            match grab.expect("grab").resize((500.0 + moved, 0.0)) {
                Command::ResizeChat { share, .. } => share,
                other => panic!("expected a chat resize, got {other:?}"),
            }
        };
        let left = share(SidebarSide::Left, 100.0);
        let right = share(SidebarSide::Right, -100.0);
        assert!((left - 0.55).abs() < 1e-3, "{left}");
        assert!((right - 0.55).abs() < 1e-3, "{right}");
    }
}
