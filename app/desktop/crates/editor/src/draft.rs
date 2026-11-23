use serde::{Deserialize, Serialize};

/// The unsaved text of a dirty document, kept so a restart or a crash loses nothing, as
/// develop's `EditorDraft`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Draft {
    /// The document, as [`crate::workspace_uri`] or [`crate::host_uri`] names it.
    pub uri: String,
    /// The text the document held.
    pub text: String,
    /// The text its file held when the draft was written.
    pub saved_text: String,
}

impl Draft {
    /// Whether the draft holds edits that are on neither side: not its saved text, and not
    /// `disk`, as develop's `unsaved`.
    #[must_use]
    pub fn unsaved(
        &self,
        disk: Option<&str>,
    ) -> bool {
        let on_disk = disk.unwrap_or(&self.saved_text);
        self.text != self.saved_text && self.text != on_disk
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn draft(
        text: &str,
        saved_text: &str,
    ) -> Draft {
        Draft {
            uri: "crowbar://host/%2Fa".to_owned(),
            text: text.to_owned(),
            saved_text: saved_text.to_owned(),
        }
    }

    #[test]
    fn a_draft_is_unsaved_only_while_its_text_is_on_neither_side() {
        let cases = [
            (draft("b", "a"), None, true),
            (draft("b", "a"), Some("a"), true),
            (draft("b", "a"), Some("b"), false),
            (draft("a", "a"), Some("c"), false),
        ];
        for (draft, disk, expected) in cases {
            assert_eq!(draft.unsaved(disk), expected, "{draft:?} over {disk:?}");
        }
    }
}
