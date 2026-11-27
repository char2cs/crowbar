use gpui::{Div, HighlightStyle, InteractiveElement, ParentElement, Styled, StyledText, div};
use ui::Palette;

use crate::panels::chats::RowInput;
use crate::panels::chats::input_view;
use crate::panels::explorer::file_tree_entry::FileTreeEntry;

const HIGHLIGHT_TINT: f32 = 0.3;

pub(super) fn shown_name(
    (entry, input): (&FileTreeEntry, Option<&RowInput>),
    matched: Option<&(String, std::ops::Range<usize>)>,
    look: (&'static str, &Palette),
) -> Div {
    match (matched, input) {
        (Some((_, range)), None) => highlighted(&entry.name, range.clone(), look.1),
        _ => name_cell(entry, input, look),
    }
}

fn name_cell(
    entry: &FileTreeEntry,
    input: Option<&RowInput>,
    look: (&'static str, &Palette),
) -> Div {
    match input {
        Some(input) => div().flex_1().min_w_0().child(input_view(input, look)),
        None => div()
            .debug_selector(|| "file-name".into())
            .min_w_0()
            .truncate()
            .child(entry.name.clone()),
    }
}

fn highlighted(
    name: &str,
    range: std::ops::Range<usize>,
    palette: &Palette,
) -> Div {
    let style = HighlightStyle {
        background_color: Some(palette.accent().opacity(HIGHLIGHT_TINT)),
        ..HighlightStyle::default()
    };
    let text = StyledText::new(name.to_owned()).with_highlights([(range, style)]);
    div().min_w_0().truncate().child(text)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use sidebar::{FileKind, GitStatus};
    use ui::{Surface, Theme};

    use super::*;
    use crate::test_support::mount;

    fn named(
        cx: &mut TestAppContext,
        matched: Option<(String, std::ops::Range<usize>)>,
    ) -> bool {
        let entry = FileTreeEntry {
            path: "pricing.ts".into(),
            name: "pricing.ts".into(),
            depth: 0,
            kind: FileKind::File,
            status: GitStatus::Clean,
            counts: None,
        };
        let cx = mount(cx, move |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            shown_name((&entry, None), matched.as_ref(), ("mono", palette))
        });
        cx.debug_bounds("file-name").is_some()
    }

    #[gpui::test]
    fn a_name_with_a_search_match_is_drawn_highlighted_in_place_of_the_plain_name(
        cx: &mut TestAppContext
    ) {
        let plain = named(cx, None);
        let matched = named(cx, Some(("pri".into(), 0..3)));
        assert_eq!((plain, matched), (true, false));
    }
}
