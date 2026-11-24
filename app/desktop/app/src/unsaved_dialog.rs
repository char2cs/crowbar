use std::ops::Range;

use gpui::{
    Context, EventEmitter, FontWeight, HighlightStyle, InteractiveElement as _, IntoElement,
    ParentElement, Render, Styled, StyledText, Window, div,
};
use ui::{Button, ButtonVariant, Dialog, DialogSize, IconName, Surface, Theme};

use crate::close_choice::CloseChoice;

const BUTTON_GAP: f32 = 8.0;
const MESSAGE_SELECTOR: &str = "unsaved-message";
const LISTED_NAMES: usize = 5;

pub(crate) struct UnsavedDialog {
    message: String,
    named: Option<Range<usize>>,
}

const ASK: &str = "Do you want to save before closing?";

fn more_than_listed(names: &[String]) -> String {
    match names.len().saturating_sub(LISTED_NAMES) {
        0 => String::new(),
        more => format!(" and {more} more"),
    }
}

pub(crate) fn message_for(names: &[String]) -> String {
    match names {
        [] => format!("Some files have unsaved changes. {ASK}"),
        [only] => format!("{only} has unsaved changes. {ASK}"),
        many => {
            let shown: Vec<&str> = many.iter().take(LISTED_NAMES).map(String::as_str).collect();
            let tail = more_than_listed(many);
            let count = many.len();
            format!(
                "{count} files have unsaved changes: {}{tail}. {ASK}",
                shown.join(", ")
            )
        },
    }
}

pub(crate) fn named_in(names: &[String]) -> Option<Range<usize>> {
    match names {
        [only] => Some(0..only.len()),
        _ => None,
    }
}

impl EventEmitter<CloseChoice> for UnsavedDialog {}

impl UnsavedDialog {
    pub(crate) fn new(names: &[String]) -> Self {
        Self {
            message: message_for(names),
            named: named_in(names),
        }
    }
}

impl Render for UnsavedDialog {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let choose = |choice: CloseChoice| cx.listener(move |_, _, _, cx| cx.emit(choice));
        let cancel = Button::new("unsaved-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(choose(CloseChoice::Cancel));
        let discard = Button::new("unsaved-discard", "Discard")
            .variant(ButtonVariant::Destructive)
            .on_click(choose(CloseChoice::Discard));
        let save = Button::new("unsaved-save", "Save").on_click(choose(CloseChoice::Save));
        let dialog = cx.entity();
        let medium = HighlightStyle {
            font_weight: Some(FontWeight::MEDIUM),
            ..HighlightStyle::default()
        };
        let named = self.named.clone().map(|range| (range, medium));
        let message = StyledText::new(self.message.clone()).with_highlights(named);
        Dialog::new("unsaved-changes")
            .title("Unsaved Changes")
            .icon(IconName::Warning)
            .size(DialogSize::Sm)
            .body(
                div()
                    .debug_selector(|| MESSAGE_SELECTOR.to_owned())
                    .text_size(ui::text_size::SM)
                    .line_height(ui::text_size::LINE_HEIGHT)
                    .text_color(palette.foreground())
                    .child(message),
            )
            .footer(
                div()
                    .flex()
                    .gap(gpui::px(BUTTON_GAP))
                    .child(cancel)
                    .child(discard)
                    .child(save),
            )
            .on_close(move |_, cx| dialog.update(cx, |_, cx| cx.emit(CloseChoice::Cancel)))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[test]
    fn the_question_names_the_file_or_counts_the_files() {
        let names = |list: &[&str]| list.iter().map(ToString::to_string).collect::<Vec<_>>();
        let cases = [
            (
                names(&["a.rs"]),
                "a.rs has unsaved changes. Do you want to save before closing?",
            ),
            (
                names(&["a.rs", "b.rs"]),
                "2 files have unsaved changes: a.rs, b.rs. Do you want to save before closing?",
            ),
            (
                names(&["1", "2", "3", "4", "5", "6", "7"]),
                "7 files have unsaved changes: 1, 2, 3, 4, 5 and 2 more. Do you want to save before closing?",
            ),
        ];
        for (list, expected) in cases {
            assert_eq!(message_for(&list), expected);
        }
    }

    #[gpui::test]
    fn the_question_is_set_in_develop_s_text_sm_lines(cx: &mut TestAppContext) {
        cx.update(|cx| cx.set_global(Theme::default()));
        let names = ["README.md".to_owned()];
        let (_, window) = cx.add_window_view(|_, _| UnsavedDialog::new(&names));
        window.run_until_parked();
        let height = window
            .debug_bounds(MESSAGE_SELECTOR)
            .map(|bounds| bounds.size.height);
        let lines = height.map(|height| height / ui::text_size::LINE_HEIGHT);
        assert!(
            lines.is_some_and(|lines| lines >= 1.0 && lines.fract() == 0.0),
            "{height:?}"
        );
    }

    #[test]
    fn a_single_file_s_name_is_set_in_medium_weight_as_develop_s_dialog_sets_it() {
        let names = |list: &[&str]| list.iter().map(ToString::to_string).collect::<Vec<_>>();
        let cases = [
            (names(&["pricing.ts"]), Some("pricing.ts")),
            (names(&["a.rs", "b.rs"]), None),
            (names(&[]), None),
        ];
        for (list, expected) in cases {
            let message = message_for(&list);
            let named = named_in(&list).and_then(|range| message.get(range));
            assert_eq!(named, expected, "{list:?}");
        }
    }
}
