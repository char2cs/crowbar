//! A titled group of rebindable shortcuts.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};

use crate::frame::Frame;
use crate::shortcut_group::ShortcutGroup;
use crate::shortcut_row::shortcut_row;
use crate::title_block::title_block;

pub(crate) fn shortcuts_view(
    frame: &Frame<'_>,
    group: &ShortcutGroup,
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let shown: Vec<_> = group
        .shortcuts
        .iter()
        .filter(|shortcut| frame.state.shortcut_visible(shortcut))
        .collect();
    if shown.is_empty() {
        return None;
    }
    let mut rows: Vec<AnyElement> = Vec::with_capacity(shown.len());
    for shortcut in shown {
        rows.push(
            shortcut_row(frame, (group, shortcut), (&mut *window, &mut *cx)).into_any_element(),
        );
    }
    let title = title_block(&group.title, None, cx);
    Some(
        div()
            .px(px(4.0))
            .child(title)
            .child(div().flex().flex_col().children(rows))
            .into_any_element(),
    )
}

#[cfg(test)]
#[path = "shortcuts_view_tests.rs"]
mod tests;
