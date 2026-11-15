use gpui::{ParentElement, SharedString, Styled, div, px};
use ui::{Kbd, Palette, radius};

use crate::emphasis::Emphasis;
use crate::ranked_entry::RankedEntry;
use crate::row_state::RowState;

const ROW_HEIGHT: f32 = 32.0;
const ROW_PADDING: f32 = 10.0;

pub(crate) fn title_runs(
    title: &str,
    positions: &[usize],
) -> Vec<(SharedString, Emphasis)> {
    let mut runs: Vec<(String, Emphasis)> = Vec::new();
    for (index, character) in title.chars().enumerate() {
        push_char(&mut runs, character, Emphasis::at(index, positions));
    }
    runs.into_iter()
        .map(|(text, emphasis)| (text.into(), emphasis))
        .collect()
}

fn push_char(
    runs: &mut Vec<(String, Emphasis)>,
    character: char,
    emphasis: Emphasis,
) {
    match runs.last_mut() {
        Some((text, same)) if *same == emphasis => text.push(character),
        _ => runs.push((character.to_string(), emphasis)),
    }
}

pub(crate) fn row(
    hit: &RankedEntry,
    state: RowState,
    palette: &Palette,
) -> gpui::Div {
    let title = title_runs(&hit.entry.title, &hit.positions)
        .into_iter()
        .fold(div().flex().flex_row(), |line, (text, emphasis)| {
            line.child(div().font_weight(emphasis.weight()).child(text))
        });
    div()
        .flex()
        .flex_row()
        .items_center()
        .justify_between()
        .h(px(ROW_HEIGHT))
        .px(px(ROW_PADDING))
        .rounded(radius::MD)
        .bg(state.fill(palette))
        .child(title)
        .children(hit.entry.shortcut.clone().map(Kbd::new))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matched_letters_split_the_title_into_runs() {
        let runs = title_runs("New tab", &[0, 4]);
        let flat: Vec<(&str, Emphasis)> = runs
            .iter()
            .map(|(text, emphasis)| (text.as_ref(), *emphasis))
            .collect();
        assert_eq!(
            flat,
            vec![
                ("N", Emphasis::Matched),
                ("ew ", Emphasis::Plain),
                ("t", Emphasis::Matched),
                ("ab", Emphasis::Plain)
            ]
        );
    }

    #[test]
    fn no_positions_is_one_plain_run() {
        let runs = title_runs("Quit", &[]);
        assert_eq!(runs.len(), 1);
    }
}
