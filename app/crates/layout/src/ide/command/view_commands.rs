use protocol::PaneId;

use crate::error::Result;
use crate::ide::command::pin_commands::group_of;
use crate::ide::presentation::editor_view::EditorView;
use crate::layout::Layout;

pub(crate) fn set_editor_view(
    layout: &mut Layout,
    pane: &PaneId,
    view: EditorView,
) -> Result<()> {
    group_of(layout, pane)?.editor_open = view == EditorView::Open;
    Ok(())
}

pub(crate) fn resize_chat(
    layout: &mut Layout,
    pane: &PaneId,
    share: f32,
) -> Result<()> {
    group_of(layout, pane)?.chat_share = Some(share);
    Ok(())
}

#[cfg(test)]
mod tests {
    use crate::error::Error;
    use crate::ide::presentation::Presentation;
    use crate::ide::presentation::editor_view::EditorView;
    use crate::ide::presentation::sidebar_side::SidebarSide;
    use crate::layout::command::Command;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, chat, layout, open};
    use crate::test_support::tab_fixtures::id;
    use crate::tree::geometry::rect::Rect;

    const AREA: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 1200.0,
        height: 800.0,
    };

    fn set(view: EditorView) -> Command {
        let pane = id("a");
        Command::SetEditorView { pane, view }
    }

    fn toggled(steps: &[EditorView]) -> Presentation {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        for command in steps.iter().map(|view| set(*view)) {
            layout.apply(command).unwrap();
        }
        let tab = layout.tab(TabId(1)).unwrap();
        let group = tab.group_of(&id("a")).unwrap();
        Presentation::of(group, AREA, SidebarSide::Left)
    }

    #[test]
    fn opening_the_editor_shows_an_empty_editor_beside_the_chat_and_closing_it_hides_it() {
        let cases = [
            ("a chat alone fills its group", vec![], false),
            ("opened, the editor shows", vec![EditorView::Open], true),
            (
                "closed again, it hides",
                vec![EditorView::Open, EditorView::Closed],
                false,
            ),
        ];
        for (name, steps, editor) in cases {
            let shown = toggled(&steps);
            assert_eq!(shown.editor.is_some(), editor, "{name}");
            assert!(shown.chat.is_some(), "{name}: the chat stays");
        }
    }

    #[test]
    fn a_resized_chat_takes_that_share_of_its_group_beside_the_editor() {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        layout.apply(set(EditorView::Open)).unwrap();
        let command = Command::ResizeChat {
            pane: id("a"),
            share: 0.6,
        };
        layout.apply(command).unwrap();
        let group = layout.tab(TabId(1)).unwrap().group_of(&id("a")).unwrap();
        let shown = Presentation::of(group, AREA, SidebarSide::Left);
        let chat = shown.chat.expect("the chat shows");
        assert!((chat.width - 716.4).abs() < 0.1, "{}", chat.width);
    }

    #[test]
    fn an_unknown_pane_is_refused() {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        let command = Command::SetEditorView {
            pane: id("zz"),
            view: EditorView::Open,
        };
        assert_eq!(layout.apply(command), Err(Error::PaneNotFound(id("zz"))));
    }
}
