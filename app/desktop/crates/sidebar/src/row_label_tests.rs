use gpui::TestAppContext;
use ui::{Surface, Theme};

use super::*;
use crate::test_support::{chat, measure};
use crate::workspace::Workspace;
use crate::workspace_status::WorkspaceStatus;

fn height(cx: &mut TestAppContext, row: Row) -> f32 {
    let bounds = measure(cx, move |_, cx| {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        label(&row, None, palette)
    });
    f32::from(bounds.size.height)
}

#[gpui::test]
fn a_row_with_a_workspace_shows_a_second_line(cx: &mut TestAppContext) {
    let plain = height(cx, chat("Refactor"));
    let mut two = Row::new("b", RowKind::Branch, 1, "Branch");
    two.workspace = Some(Workspace {
        status: WorkspaceStatus::New,
        branch: "feature".into(),
        added: 1,
        deleted: 0,
        held_by_path: None,
    });
    assert!(height(cx, two) > plain);
}
