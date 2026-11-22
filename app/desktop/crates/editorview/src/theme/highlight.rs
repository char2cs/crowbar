//! develop's editor colours, matched per token class (D3), as gpui-kit's highlight style.

use gpui_kit::component::highlighter::HighlightThemeStyle;
use serde_json::{Map, Value, json};
use ui::Palette;

const ACTIVE_LINE_ALPHA: f32 = 0.08;

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
        ("title", palette.syntax_markdown_heading()),
        ("emphasis.strong", palette.syntax_markdown_bold()),
        ("emphasis", palette.syntax_markdown_italic()),
        ("link_uri", palette.syntax_markdown_link()),
        ("link_text", palette.syntax_markdown_link_text()),
        ("text.literal", palette.syntax_markdown_code()),
        ("punctuation.list_marker", palette.syntax_markdown_list()),
    ];
    classes
        .into_iter()
        .map(|(capture, color)| (capture.to_owned(), json!({ "color": color })))
        .collect()
}

#[cfg(test)]
mod tests {
    use gpui::Hsla;
    use ui::{Surface, Theme, ThemeMode};

    use super::*;

    type Token = fn(&Palette) -> Hsla;

    #[test]
    fn each_token_class_takes_develop_s_colour_and_dotted_captures_fall_back_to_their_class() {
        const CASES: [(&str, Token); 7] = [
            ("keyword", Palette::syntax_keyword),
            ("string", Palette::syntax_string),
            ("comment", Palette::syntax_comment),
            ("function.method", Palette::syntax_function),
            ("type.builtin", Palette::syntax_type),
            ("punctuation.bracket", Palette::syntax_punctuation),
            ("title", Palette::syntax_markdown_heading),
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
}
