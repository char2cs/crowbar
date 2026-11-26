//! develop's editor colours, matched per token class (D3), as gpui-kit's highlight style.
//!
//! Markdown takes the classes develop's Monaco grammar gives it, not the `--syntax-markdown-*`
//! tokens, which develop's editor never reads: headings and list markers are keywords, link
//! targets and code blocks strings, inline code a variable, and bold and italic keep the text
//! colour.

use gpui_kit::component::highlighter::HighlightThemeStyle;
use serde_json::{Map, Value, json};
use ui::Palette;

const ACTIVE_LINE_ALPHA: f32 = 0.08;
const BOLD: u16 = 700;

pub(super) fn highlight_style(palette: &Palette) -> Result<HighlightThemeStyle, serde_json::Error> {
    let mut active_line = palette.border();
    active_line.a = ACTIVE_LINE_ALPHA;
    serde_json::from_value(json!({
        "editor.background": palette.pane_background(),
        "editor.foreground": palette.foreground(),
        "editor.active_line.background": active_line,
        "editor.line_number": palette.muted_foreground(),
        "editor.active_line_number": palette.foreground(),
        "syntax": Value::Object(syntax(palette)),
    }))
}

fn syntax(palette: &Palette) -> Map<String, Value> {
    let classes = [
        ("comment", palette.syntax_comment()),
        ("punctuation", palette.syntax_punctuation()),
        ("punctuation.bracket", palette.muted_foreground()),
        ("operator", palette.syntax_operator()),
        ("keyword", palette.syntax_keyword()),
        ("string", palette.syntax_string()),
        ("string.regex", palette.syntax_regex()),
        ("number", palette.syntax_number()),
        ("constant", palette.syntax_constant()),
        ("boolean", palette.syntax_boolean()),
        ("function", palette.syntax_function()),
        ("type", palette.syntax_type()),
        ("constructor", palette.syntax_type()),
        ("property", palette.syntax_property()),
        ("tag", palette.syntax_tag()),
        ("attribute", palette.syntax_attribute()),
        ("title", palette.syntax_keyword()),
        ("link_uri", palette.syntax_string()),
        ("text.literal", palette.syntax_string()),
        ("text.code.span", palette.syntax_variable()),
        ("punctuation.list_marker", palette.syntax_keyword()),
    ];
    let painted = classes
        .into_iter()
        .map(|(capture, color)| (capture.to_owned(), json!({ "color": color })));
    let faced = [
        ("emphasis.strong", json!({ "font_weight": BOLD })),
        ("emphasis", json!({ "font_style": "italic" })),
    ]
    .into_iter()
    .map(|(capture, face)| (capture.to_owned(), face));
    painted.chain(faced).collect()
}

#[cfg(test)]
mod tests {
    use gpui::{FontStyle, FontWeight, HighlightStyle, Hsla};
    use gpui_kit::base::input::HighlightStyleResolver;
    use gpui_kit::component::Rope;
    use gpui_kit::component::highlighter::SyntaxHighlighter;
    use ui::{Surface, Theme, ThemeMode};

    use super::*;

    type Token = fn(&Palette) -> Hsla;

    #[test]
    fn each_token_class_takes_develop_s_colour_and_dotted_captures_fall_back_to_their_class() {
        const CASES: [(&str, Token); 10] = [
            ("keyword", Palette::syntax_keyword),
            ("string", Palette::syntax_string),
            ("comment", Palette::syntax_comment),
            ("function.method", Palette::syntax_function),
            ("type.builtin", Palette::syntax_type),
            ("punctuation.bracket", Palette::muted_foreground),
            ("title", Palette::syntax_keyword),
            ("punctuation.list_marker", Palette::syntax_keyword),
            ("link_uri", Palette::syntax_string),
            ("text.literal", Palette::syntax_string),
        ];
        let modes = [ThemeMode::Dark, ThemeMode::Light];
        for (mode, (capture, token)) in modes
            .into_iter()
            .flat_map(|mode| CASES.map(|case| (mode, case)))
        {
            let palette = Theme::default().with_mode(mode).palette(Surface::Window);
            let painted = highlight_style(palette).unwrap().syntax.style(capture);
            assert_eq!(
                painted.and_then(|style| style.color),
                Some(token(palette)),
                "{mode:?} {capture}"
            );
        }
    }

    struct Painted(HighlightThemeStyle);

    impl HighlightStyleResolver for Painted {
        fn style(
            &self,
            name: &str,
        ) -> Option<HighlightStyle> {
            self.0.syntax.style(name)
        }
    }

    fn styles_of(
        (language, text): (&str, &str),
        palette: &Palette,
    ) -> Vec<(std::ops::Range<usize>, HighlightStyle)> {
        assert!(crate::theme::grammars::register().is_ok());
        let mut highlighter = SyntaxHighlighter::new(language);
        highlighter.update(None, &Rope::from(text), None);
        let painted = Painted(highlight_style(palette).unwrap());
        highlighter.styles(&(0..text.len()), &painted)
    }

    fn style_at(
        styles: &[(std::ops::Range<usize>, HighlightStyle)],
        offset: usize,
    ) -> HighlightStyle {
        styles
            .iter()
            .rev()
            .find(|(range, _)| range.contains(&offset))
            .map_or_else(HighlightStyle::default, |(_, style)| *style)
    }

    #[test]
    fn markdown_bold_and_italic_keep_the_text_colour_as_develop_s_editor_draws_them() {
        let palette = Theme::default()
            .with_mode(ThemeMode::Dark)
            .palette(Surface::Window);
        let styles = styles_of(("markdown", "**bold** and *it*\n"), palette);
        let (bold, italic) = (style_at(&styles, 3), style_at(&styles, 15));
        assert_eq!(
            (
                bold.color,
                bold.font_weight,
                italic.color,
                italic.font_style
            ),
            (None, Some(FontWeight::BOLD), None, Some(FontStyle::Italic))
        );
    }

    #[test]
    fn headings_builtin_types_and_inline_code_take_develop_s_classes() {
        const HEADINGS: &str = "# checkout\n\nTitle\n=====\n";
        const CODE: &str = "a `code` b\n";
        let keyword: Option<Token> = Some(Palette::syntax_keyword);
        let rows = [
            (("markdown", HEADINGS), 0, keyword),
            (("markdown", HEADINGS), 2, keyword),
            (("markdown", HEADINGS), 12, None),
            (("markdown", HEADINGS), 18, keyword),
            (("typescript", "let a: string = b\n"), 7, keyword),
            (("markdown", CODE), 4, Some(Palette::syntax_variable)),
            (("markdown", CODE), 2, Some(Palette::syntax_variable)),
        ];
        let modes = [ThemeMode::Dark, ThemeMode::Light];
        let cases = modes
            .into_iter()
            .flat_map(|mode| rows.map(|row| (mode, row)));
        for (mode, (source, offset, token)) in cases {
            let palette = Theme::default().with_mode(mode).palette(Surface::Window);
            let color = style_at(&styles_of(source, palette), offset).color;
            let expected = token.map(|token| token(palette));
            assert_eq!(color, expected, "{mode:?} {source:?} at {offset}");
        }
    }
}
