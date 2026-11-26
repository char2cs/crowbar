//! Monaco's bracket pair colour over one view's text, on every bracket outside strings and
//! comments.

mod quiet_captures;

use gpui::{App, Context, Entity, HighlightStyle, Subscription};
use gpui_kit::base::input::{InputEditsEvent, Rope, TextDecoration, TextDecorationCollection};
use gpui_kit::component::highlighter::SyntaxHighlighter;
use gpui_kit::component::input::EditorState;
use quiet_captures::QuietCaptures;
use ui::{Palette, Surface, Theme};

pub(super) struct BracketColours {
    state: Entity<EditorState>,
    highlighter: SyntaxHighlighter,
    painted: TextDecorationCollection,
    _edits: Subscription,
    _theme: Subscription,
}

impl BracketColours {
    pub(super) fn new(
        state: &Entity<EditorState>,
        language: &str,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let state = state.clone();
        let painted = state.update(cx, |state, cx| {
            state.create_decorations_collection(vec![], cx)
        });
        let edits = cx.subscribe(&state, |colours, _, event: &InputEditsEvent, cx| {
            colours.follow(event, cx);
        });
        let theme = cx.observe_global::<Theme>(|colours, cx| colours.paint(cx));
        let mut colours = Self {
            state,
            highlighter: SyntaxHighlighter::new(language),
            painted,
            _edits: edits,
            _theme: theme,
        };
        colours.relanguage(language, cx);
        colours
    }

    pub(super) fn relanguage(
        &mut self,
        language: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.highlighter = SyntaxHighlighter::new(language);
        let text = self.state.read(cx).text().clone();
        self.highlighter.update(None, &text, None);
        self.paint(cx);
    }

    fn follow(
        &mut self,
        event: &InputEditsEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((_, text)) = event.edits().last() else {
            return;
        };
        self.highlighter.update(None, text, None);
        self.paint(cx);
    }

    fn paint(
        &self,
        cx: &mut App,
    ) {
        let palette = Theme::of(cx).palette(Surface::Window);
        let decorations = decorations(&self.highlighter, palette);
        self.painted.set(decorations, cx);
    }
}

fn decorations(
    highlighter: &SyntaxHighlighter,
    palette: &Palette,
) -> Vec<TextDecoration> {
    let text: &Rope = highlighter.text();
    let quiet: Vec<_> = highlighter
        .styles(&(0..text.len()), &QuietCaptures::STRINGS_AND_COMMENTS)
        .into_iter()
        .filter(|(_, style)| QuietCaptures::is_quiet(style))
        .map(|(range, _)| range)
        .collect();
    let style = HighlightStyle {
        color: Some(palette.muted_foreground()),
        ..HighlightStyle::default()
    };
    editor::brackets(&text.to_string(), &quiet)
        .into_iter()
        .map(|offset| TextDecoration::new(offset..offset + 1, style))
        .collect()
}

#[cfg(test)]
mod tests {
    use gpui::Hsla;
    use ui::ThemeMode;

    use super::*;

    type Token = fn(&Palette) -> Hsla;
    type Case = ((&'static str, &'static str), usize, Option<Token>);

    fn colour_at(
        (language, text): (&str, &str),
        offset: usize,
        palette: &Palette,
    ) -> Option<Hsla> {
        let mut highlighter = SyntaxHighlighter::new(language);
        highlighter.update(None, &Rope::from(text), None);
        decorations(&highlighter, palette)
            .into_iter()
            .find(|decoration| decoration.range.start == offset)
            .and_then(|decoration| decoration.style.color)
    }

    #[test]
    fn a_bracket_takes_monaco_s_pair_colour_outside_strings_and_comments() {
        let cases: [Case; 6] = [
            (
                ("markdown", "a `f()` b\n"),
                4,
                Some(Palette::muted_foreground),
            ),
            (
                ("markdown", "a `f()` b\n"),
                5,
                Some(Palette::muted_foreground),
            ),
            (("markdown", "[a](b(c))\n"), 5, None),
            (
                ("typescript", "f(\"(\")\n"),
                1,
                Some(Palette::muted_foreground),
            ),
            (("typescript", "f(\"(\")\n"), 3, None),
            (("typescript", "// (\n"), 3, None),
        ];
        let modes = [ThemeMode::Dark, ThemeMode::Light];
        for (mode, (source, offset, token)) in modes
            .into_iter()
            .flat_map(|mode| cases.map(|case| (mode, case)))
        {
            let palette = Theme::default().with_mode(mode).palette(Surface::Window);
            let expected = token.map(|token| token(palette));
            let found = colour_at(source, offset, palette);
            assert_eq!(found, expected, "{mode:?} {source:?} at {offset}");
        }
    }
}
