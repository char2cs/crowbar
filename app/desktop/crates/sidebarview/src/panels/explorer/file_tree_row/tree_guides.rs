use gpui::{Div, InteractiveElement, Styled, div, px};
use sidebar::IndentGuides;
use ui::Palette;

use crate::panels::explorer::file_tree_row::TreeLook;

const BASE_INDENT: f32 = 6.0;
const ICON_OFFSET: f32 = 7.0;
const ROW_BORDER: f32 = 1.0;
const END_INSET: f32 = 4.0;
const GUIDE_WIDTH: f32 = 1.0;
const GUIDE_ALPHA: f32 = 0.18 * 0.9;

pub(crate) fn tree_guides(
    placed: (u8, (u8, u8)),
    (look, palette): (&TreeLook, &Palette),
) -> Vec<Div> {
    let (depth, (above, below)) = placed;
    if look.guides == IndentGuides::Hidden {
        return Vec::new();
    }
    let color = palette.muted_foreground().opacity(GUIDE_ALPHA);
    (0..depth)
        .map(|level| {
            let left = BASE_INDENT + f32::from(level) * look.indent + ICON_OFFSET - ROW_BORDER;
            div()
                .debug_selector(move || format!("tree-guide-{level}"))
                .absolute()
                .left(px(left))
                .top(px(inset(above, level) - ROW_BORDER))
                .bottom(px(inset(below, level) - ROW_BORDER))
                .w(px(GUIDE_WIDTH))
                .bg(color)
        })
        .collect()
}

fn inset(
    neighbour: u8,
    level: u8,
) -> f32 {
    if neighbour <= level {
        return END_INSET;
    }
    0.0
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use sidebar::{FileKind, GitStatus};

    use crate::panels::explorer::file_tree_entry::FileTreeEntry;
    use crate::panels::explorer::file_tree_row::{FileTreeRow, TreeLook};
    use crate::test_support::mount;

    #[gpui::test]
    fn a_nested_row_draws_one_guide_per_level_inset_where_its_level_ends(cx: &mut TestAppContext) {
        let nested = FileTreeEntry {
            depth: 2,
            path: "src/lib/a.rs".into(),
            name: "a.rs".into(),
            kind: FileKind::File,
            status: GitStatus::Clean,
            counts: None,
        };
        let look = TreeLook::default();
        let indent = look.indent;
        let cx = mount(cx, move |_, _| FileTreeRow {
            entry: nested.clone(),
            neighbours: (2, 1),
            input: None,
            selected: false,
            cut: false,
            note: None,
            target: None,
            look: look.clone(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::new(|_, _, _| {}),
        });
        let probe = cx.debug_bounds("probe").unwrap_or_default();
        let guides = ["tree-guide-0", "tree-guide-1", "tree-guide-2"].map(|selector| {
            let bounds = cx.debug_bounds(selector)?;
            let left = f32::from(bounds.left() - probe.left());
            let insets = (bounds.top() - probe.top(), probe.bottom() - bounds.bottom());
            Some((left, f32::from(insets.0), f32::from(insets.1)))
        });
        let row_inset = 6.0;
        assert_eq!(
            guides,
            [
                Some((row_inset + 13.0, 0.0, 0.0)),
                Some((row_inset + 13.0 + indent, 0.0, 4.0)),
                None
            ]
        );
    }
}
