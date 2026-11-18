//! A titled group of rebindable shortcuts.

mod row;

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div};
use settings::ShortcutGroup;

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::pages::shortcuts::row::shortcut_row;
use crate::modal::pages::title_block::title_block;

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
            .px(spacing::TIGHT)
            .child(title)
            .child(div().flex().flex_col().children(rows))
            .into_any_element(),
    )
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use ui::TextInputEvent;

    fn visible_shortcuts(modal: &crate::settings_modal::SettingsModal) -> usize {
        modal
            .state
            .schema()
            .shortcuts()
            .filter(|(_, shortcut)| modal.state.shortcut_visible(shortcut))
            .count()
    }

    #[gpui::test]
    fn a_search_keeps_only_the_matching_shortcuts(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "keybindings");
        let count = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
            inspect(harness, cx, visible_shortcuts)
        };
        let all = count(&harness, cx);
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("console".to_owned())),
            cx,
        );
        assert_eq!((all, count(&harness, cx)), (24, 1));
    }
}
