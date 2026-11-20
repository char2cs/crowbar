//! A titled group of rebindable shortcuts.

pub(crate) mod capture;
mod chord_face;
mod chord_look;
mod row;

use gpui::prelude::FluentBuilder as _;
use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div};
use settings::ShortcutGroup;
use ui::{Surface, Theme};

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::pages::shortcuts::row::shortcut_row;
use crate::modal::pages::title_block::title_block;

const DIVIDER_FADE: f32 = 0.6;

pub(crate) fn shortcuts_view(
    frame: &Frame<'_>,
    group: &ShortcutGroup,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    let shortcuts = group.shortcuts();
    let divider = Theme::of(cx)
        .palette(Surface::Window)
        .border()
        .opacity(DIVIDER_FADE);
    let mut rows: Vec<AnyElement> = Vec::with_capacity(shortcuts.len());
    for (index, shortcut) in shortcuts.iter().enumerate() {
        let row = shortcut_row(frame, (group, shortcut), (&mut *window, &mut *cx));
        let ruled = div().when(index > 0, |d| d.border_t_1().border_color(divider));
        rows.push(ruled.child(row).into_any_element());
    }
    let title = title_block(group.title(), None, cx);
    div()
        .px(spacing::TIGHT)
        .child(title)
        .child(div().flex().flex_col().children(rows))
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::open;

    #[gpui::test]
    fn every_group_draws_a_row_per_shortcut(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "keybindings");
        cx.simulate_resize(gpui::size(gpui::px(1600.0), gpui::px(4000.0)));
        cx.run_until_parked();
        for selector in [
            "shortcut-navigation.toggleConsole",
            "shortcut-tabs.openFile",
            "shortcut-editor.saveAll",
        ] {
            assert!(cx.debug_bounds(selector).is_some(), "{selector}");
        }
    }

    #[gpui::test]
    fn a_chord_button_stands_as_tall_as_develops_outline_button(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "keybindings");
        let chord = cx.debug_bounds("chord-navigation.toggleConsole");
        assert_eq!(chord.map(|bounds| bounds.size.height), Some(gpui::px(32.0)));
    }
}
