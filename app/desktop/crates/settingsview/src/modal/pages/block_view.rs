//! Draws one block of a page, or nothing when the search hides it.

use gpui::{AnyElement, App, IntoElement, Window};
use settings::Block;

use crate::frame::Frame;
use crate::modal::pages::button_block::button_block;
use crate::modal::pages::heading_view::heading_view;
use crate::modal::pages::records::list_view;
use crate::modal::pages::section_view::section_view;
use crate::modal::pages::shortcuts::shortcuts_view;

pub(crate) fn block_view(
    frame: &Frame<'_>,
    entry: (usize, &Block),
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let (index, block) = entry;
    match block {
        Block::Section(section) => section_view(frame, section, (window, cx)),
        _ if !frame.state.block_visible(index) => None,
        Block::Shortcuts(group) => Some(shortcuts_view(frame, group, (window, cx))),
        Block::Heading(heading) => Some(heading_view(heading, cx).into_any_element()),
        Block::List(list) => Some(list_view(frame, list, (window, cx)).into_any_element()),
        Block::Button(spec) => button_block(frame, spec, cx).map(IntoElement::into_any_element),
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use ui::TextInputEvent;

    fn visible_blocks(modal: &crate::settings_modal::SettingsModal) -> usize {
        let count = modal.state.page().map_or(0, |page| page.blocks().len());
        (0..count)
            .filter(|&index| modal.state.block_visible(index))
            .count()
    }

    #[gpui::test]
    fn the_agent_lists_show_with_the_permission_row_and_hide_without_it(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let mut shown = |query: &str| {
            let typed = Intent::Search(TextInputEvent::Changed(query.to_owned()));
            run(&harness, typed, cx);
            inspect(&harness, cx, visible_blocks)
        };
        assert_eq!(shown("permission"), 4, "heading, upload and both lists");
        assert_eq!(shown("chat by default"), 0);
    }
}
