use protocol::PaneId;

use crate::error::Result;
use crate::ide::editor_view::EditorView;
use crate::ide::pin_commands::group_of;
use crate::layout::Layout;

pub(crate) fn set_editor_view(
    layout: &mut Layout,
    pane: &PaneId,
    view: EditorView,
) -> Result<()> {
    group_of(layout, pane)?.editor_open = view == EditorView::Open;
    Ok(())
}

#[cfg(test)]
mod tests {
    use crate::command::Command;
    use crate::error::Error;
    use crate::ide::editor_view::EditorView;
    use crate::ide::presentation::{DEFAULT_CHAT_SHARE, Presentation};
    use crate::ide::sidebar_side::SidebarSide;
    use crate::rect::Rect;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, chat, layout, open};
    use crate::test_support::tab_fixtures::id;

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
        Presentation::of(group, AREA, SidebarSide::Left, DEFAULT_CHAT_SHARE)
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
    fn an_unknown_pane_is_refused() {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        let command = Command::SetEditorView {
            pane: id("zz"),
            view: EditorView::Open,
        };
        assert_eq!(layout.apply(command), Err(Error::PaneNotFound(id("zz"))));
    }
}
