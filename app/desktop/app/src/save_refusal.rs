const SAVE_FAILED: &str = "Failed to save file";
const READ_ONLY: &str = "Workspace is read-only";
const LOCKED: &str = "locked";
const READ_ONLY_HINT: &str = "Create or switch to a child workspace to edit files.";

pub(crate) fn words(reason: String) -> (&'static str, String) {
    if reason.to_lowercase().contains(LOCKED) {
        return (READ_ONLY, READ_ONLY_HINT.to_owned());
    }
    (SAVE_FAILED, reason)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_locked_workspace_says_it_is_read_only_and_any_other_refusal_says_why() {
        let cases = [
            (
                "workspace is locked",
                (READ_ONLY, READ_ONLY_HINT.to_owned()),
            ),
            ("Locked: main", (READ_ONLY, READ_ONLY_HINT.to_owned())),
            ("disk full", (SAVE_FAILED, "disk full".to_owned())),
        ];
        for (reason, expected) in cases {
            assert_eq!(words(reason.to_owned()), expected, "{reason}");
        }
    }
}
