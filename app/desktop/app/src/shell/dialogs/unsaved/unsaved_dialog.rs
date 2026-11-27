use crate::shell::dialogs::unsaved::close_choice::CloseChoice;

const LISTED_NAMES: usize = 5;

const ASK: &str = "Do you want to save before closing?";
const TITLE: &str = "Unsaved Changes";

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

pub(crate) fn native_request(names: &[String]) -> webview::AlertRequest {
    let button = |label: &str, role| webview::AlertButton {
        label: label.to_owned(),
        role,
    };
    webview::AlertRequest {
        title: Some(TITLE.to_owned()),
        message: message_for(names),
        buttons: vec![
            button("Cancel", webview::AlertRole::Cancel),
            button("Discard", webview::AlertRole::Destructive),
            button("Save", webview::AlertRole::Accept),
        ],
    }
}

pub(crate) const fn choice_of(pressed: Option<usize>) -> CloseChoice {
    match pressed {
        Some(1) => CloseChoice::Discard,
        Some(2) => CloseChoice::Save,
        _ => CloseChoice::Cancel,
    }
}

#[cfg(test)]
mod tests {
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
}
