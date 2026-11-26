//! The question asked before reloading a tab whose edits are unsaved.

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/tabs/components/tab-bar-context-menu.tsx
pub(crate) fn message_for(name: &str) -> String {
    format!("Discard unsaved changes to {name} and reload it from disk?")
}

pub(crate) const OK: Option<usize> = Some(1);

pub(crate) fn native_request(name: &str) -> webview::AlertRequest {
    let button = |label: &str, role| webview::AlertButton {
        label: label.to_owned(),
        role,
    };
    webview::AlertRequest {
        title: None,
        message: message_for(name),
        buttons: vec![
            button("Cancel", webview::AlertRole::Cancel),
            button("OK", webview::AlertRole::Accept),
        ],
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_question_names_the_file() {
        assert_eq!(
            message_for("README.md"),
            "Discard unsaved changes to README.md and reload it from disk?"
        );
    }
}
