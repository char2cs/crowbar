use gpui::Context;
use gpui::Hsla;
use gpui_kit::component::input::{RangeDecoration, RangeDecorationStyle};
use ui::{Palette, Surface, Theme};

use super::FindWidget;

impl FindWidget {
    pub(super) fn paint(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let current = self.current;
        let decorations = self
            .matches
            .iter()
            .enumerate()
            .map(|(index, range)| {
                RangeDecoration::new(range.clone())
                    .with_style(RangeDecorationStyle::Fill)
                    .with_color(match_color(index, current, &palette))
            })
            .collect();
        self.highlights.set(decorations, cx);
        cx.notify();
    }
}

fn match_color(
    index: usize,
    current: Option<usize>,
    palette: &Palette,
) -> Hsla {
    if Some(index) == current {
        return palette.editor_selection();
    }
    palette.border()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[gpui::test]
    fn the_current_match_takes_the_selection_colour_and_the_rest_the_border(
        cx: &mut gpui::TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        let palette = cx.update(|cx| *Theme::of(cx).palette(Surface::Window));
        let cases = [
            (0, Some(0), palette.editor_selection()),
            (1, Some(0), palette.border()),
            (0, None, palette.border()),
        ];
        for (index, current, expected) in cases {
            assert_eq!(
                match_color(index, current, &palette),
                expected,
                "{index} {current:?}"
            );
        }
    }
}
