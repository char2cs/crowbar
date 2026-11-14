//! Keyboard highlight movement over the rows of a menu.

use crate::direction::Direction;
use crate::menu_item::MenuItem;

pub(crate) fn step(items: &[MenuItem], from: Option<usize>, direction: Direction) -> Option<usize> {
    let count = items.len();
    let start = match (from, direction) {
        (Some(index), Direction::Forward) => index + 1,
        (Some(index), Direction::Backward) => index + count - 1,
        (None, Direction::Forward) => 0,
        (None, Direction::Backward) => count.saturating_sub(1),
    };
    (0..count)
        .map(|offset| match direction {
            Direction::Forward => (start + offset) % count,
            Direction::Backward => (start + count - offset) % count,
        })
        .find(|&index| items.get(index).is_some_and(MenuItem::is_selectable))
}

#[cfg(test)]
#[path = "menu_cursor_tests.rs"]
mod tests;
