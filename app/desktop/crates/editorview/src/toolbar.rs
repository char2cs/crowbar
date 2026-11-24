use gpui::{
    AnyElement, App, FontWeight, Hsla, IntoElement as _, ParentElement as _, Pixels, Styled as _,
    div, px,
};
use ui::{
    Icon, IconName, MaterialIcon, Palette, ProviderGlyph, ScaledText, Surface, Theme, radius,
    text_size,
};

use crate::caret::Caret;

const SEPARATOR: char = '/';
const BAR_HEIGHT: f32 = 28.0;
const BAR_PADDING_X: f32 = 12.0;
const BAR_PADDING_Y: f32 = 4.0;
const LEFT_GAP: f32 = 8.0;
const RIGHT_GAP: f32 = 4.0;
const SEGMENT_GAP: f32 = 2.0;
const SEGMENT_HEIGHT: f32 = 32.0;
const SEGMENT_PADDING_X: f32 = 4.0;
const SEGMENT_PADDING_Y: f32 = 2.0;
const ICON_BOX: f32 = 20.0;
const ICON_SIZE: f32 = 16.0;
const CHEVRON_MARGIN_X: f32 = 2.0;
const CHIP_HEIGHT: f32 = 20.0;
const CHIP_PADDING_X: f32 = 6.0;
const CORNER: f32 = 4.0;
const DIVIDER_MARGIN_X: f32 = 4.0;
const DIVIDER_HEIGHT: f32 = 14.0;
const DIVIDER_ALPHA: f32 = 0.7;

pub(crate) struct Toolbar {
    path: String,
    language: editor::Language,
}

impl Toolbar {
    pub(crate) fn of(path: &str) -> Self {
        Self {
            path: path.to_owned(),
            language: editor::Language::of_path(path),
        }
    }

    pub(crate) fn language(&self) -> editor::Language {
        self.language
    }

    pub(crate) fn previews(&self) -> bool {
        editor::Preview::of_path(&self.path).is_some()
    }

    pub(crate) fn has_rich_view(&self) -> bool {
        editor::Preview::has_rich_view(&self.path)
    }

    pub(crate) fn element(
        &self,
        caret: Caret,
        (actions, after_language): (AnyElement, Option<AnyElement>),
        cx: &mut App,
    ) -> AnyElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let xs = ScaledText::Xs.size(Theme::of(cx));
        let divider = div()
            .mx(px(DIVIDER_MARGIN_X))
            .h(px(DIVIDER_HEIGHT))
            .w(px(1.0))
            .bg(palette.border().opacity(DIVIDER_ALPHA));
        let chips = div()
            .flex()
            .items_center()
            .gap(px(RIGHT_GAP))
            .child(actions)
            .child(divider)
            .child(
                chip(format!("{}:{}", caret.line, caret.column), &palette)
                    .text_size(xs)
                    .into_any_element(),
            )
            .child(
                chip(self.language.display_name().to_owned(), &palette)
                    .text_size(text_size::SM)
                    .font_weight(FontWeight::MEDIUM)
                    .into_any_element(),
            )
            .children(after_language);
        div()
            .flex()
            .flex_none()
            .min_h(px(BAR_HEIGHT))
            .items_center()
            .justify_between()
            .px(px(BAR_PADDING_X))
            .py(px(BAR_PADDING_Y))
            .child(self.breadcrumb(&palette, cx))
            .child(chips)
            .into_any_element()
    }

    fn breadcrumb(
        &self,
        palette: &Palette,
        cx: &mut App,
    ) -> AnyElement {
        let xs = ScaledText::Xs.size(Theme::of(cx));
        let segments = segments(&self.path);
        let file_name = segments.last().copied().unwrap_or(self.path.as_str());
        let svg = MaterialIcon::of(file_name, cx).monochrome();
        let icon = div()
            .flex()
            .flex_none()
            .size(px(ICON_BOX))
            .items_center()
            .justify_center()
            .child(ProviderGlyph::new(svg, palette.muted_foreground()).size(px(ICON_SIZE)));
        let last = segments.len().saturating_sub(1);
        let drawn = segments.iter().enumerate().map(|(index, segment)| {
            div()
                .flex()
                .flex_none()
                .items_center()
                .gap(px(SEGMENT_GAP))
                .children((index > 0).then(|| chevron(xs, palette)))
                .child(segment_label((index, segment), last, palette))
        });
        div()
            .flex()
            .min_w_0()
            .items_center()
            .gap(px(LEFT_GAP))
            .text_size(xs)
            .child(
                div()
                    .flex()
                    .min_w_0()
                    .items_center()
                    .gap(px(SEGMENT_GAP))
                    .child(icon)
                    .children(drawn),
            )
            .into_any_element()
    }
}

fn segments(path: &str) -> Vec<&str> {
    path.split(SEPARATOR)
        .filter(|segment| !segment.is_empty())
        .collect()
}

fn chevron(
    size: Pixels,
    palette: &Palette,
) -> AnyElement {
    div()
        .mx(px(CHEVRON_MARGIN_X))
        .child(
            Icon::new(IconName::CaretRight)
                .size(size)
                .color(palette.muted_foreground()),
        )
        .into_any_element()
}

fn segment_label(
    (index, segment): (usize, &str),
    last: usize,
    palette: &Palette,
) -> AnyElement {
    div()
        .flex()
        .flex_none()
        .h(px(SEGMENT_HEIGHT))
        .items_center()
        .px(px(SEGMENT_PADDING_X))
        .py(px(SEGMENT_PADDING_Y))
        .rounded(px(CORNER))
        .text_size(text_size::SM)
        .font_weight(FontWeight::MEDIUM)
        .text_color(segment_ink((index, last), palette))
        .child(segment.to_owned())
        .into_any_element()
}

fn segment_ink(
    (index, last): (usize, usize),
    palette: &Palette,
) -> Hsla {
    if index == last {
        return palette.foreground();
    }
    palette.muted_foreground()
}

fn chip(
    text: String,
    palette: &Palette,
) -> gpui::Div {
    div()
        .flex()
        .h(px(CHIP_HEIGHT))
        .items_center()
        .px(px(CHIP_PADDING_X))
        .rounded(radius::MD)
        .text_color(palette.muted_foreground())
        .child(text)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_path_shows_one_segment_per_folder_and_the_file_last() {
        let cases = [
            ("src/lib/main.rs", vec!["src", "lib", "main.rs"]),
            ("/etc/hosts", vec!["etc", "hosts"]),
            ("README.md", vec!["README.md"]),
        ];
        for (path, expected) in cases {
            assert_eq!(segments(path), expected, "{path}");
        }
    }
}
