use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Draft {
    pub uri: String,
    pub text: String,
    pub saved_text: String,
}

impl Draft {
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
