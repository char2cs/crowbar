//! Draws one block of a page, or nothing when the search hides it.

use gpui::{AnyElement, App, IntoElement, Window};
use settings::Block;

use crate::button_block::button_block;
use crate::frame::Frame;
use crate::heading_view::heading_view;
use crate::list_view::list_view;
use crate::section_view::section_view;
use crate::shortcuts_view::shortcuts_view;

pub(crate) fn block_view(
    frame: &Frame<'_>,
    entry: (usize, &Block),
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let (index, block) = entry;
    match block {
        Block::Section(section) => section_view(frame, section, (window, cx)),
        Block::Shortcuts(group) => shortcuts_view(frame, group, (window, cx)),
        _ if !frame.state.block_visible(index) => None,
        Block::Heading(heading) => Some(heading_view(heading, cx).into_any_element()),
        Block::List(list) => Some(list_view(frame, list, (window, cx)).into_any_element()),
        Block::Button(spec) => button_block(frame, spec).map(IntoElement::into_any_element),
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use ui::TextInputEvent;

    fn visible_blocks(modal: &crate::settings_modal::SettingsModal) -> usize {
        let count = modal.state.page().map_or(0, |page| page.blocks.len());
        (0..count)
            .filter(|&index| modal.state.block_visible(index))
            .count()
    }

    #[gpui::test]
    fn a_list_block_follows_the_search(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "providers");
        let any_block = |harness: &crate::test_support::Harness,
                         cx: &mut gpui::VisualTestContext| {
            inspect(harness, cx, visible_blocks)
        };
        let all = any_block(&harness, cx);
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("descriptor".to_owned())),
            cx,
        );
        assert!(any_block(&harness, cx) <= all);
    }
}
