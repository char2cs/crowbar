//! Keyboard highlight movement over the rows of a menu.

use crate::menu_item::MenuItem;

/// Index of the next selectable row after `from` in the direction of `forward`, wrapping around.
pub(crate) fn step(items: &[MenuItem], from: Option<usize>, forward: bool) -> Option<usize> {
    let count = items.len();
    let start = match (from, forward) {
        (Some(index), true) => index + 1,
        (Some(index), false) => index + count - 1,
        (None, true) => 0,
        (None, false) => count.saturating_sub(1),
    };
    (0..count)
        .map(|offset| if forward { (start + offset) % count } else { (start + count - offset) % count })
        .find(|&index| items.get(index).is_some_and(MenuItem::is_selectable))
}

#[cfg(test)]
#[path = "menu_cursor_tests.rs"]
mod tests;
