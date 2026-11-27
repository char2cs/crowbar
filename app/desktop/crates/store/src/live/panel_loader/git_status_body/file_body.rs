use serde::Deserialize;
use sidebar::GitStatus;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct FileBody {
    pub(crate) path: String,
    pub(crate) status: String,
    pub(crate) staged: bool,
}

impl FileBody {
    pub(crate) fn git_status(&self) -> GitStatus {
        match (self.status.as_str(), self.staged) {
            ("added", _) => GitStatus::Added,
            ("deleted", _) => GitStatus::Deleted,
            ("untracked", _) => GitStatus::Untracked,
            ("renamed", _) => GitStatus::Renamed,
            ("modified", true) => GitStatus::StagedModified,
            _ => GitStatus::Modified,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, GitStatus); 7] = [
        (r#"{"status":"untracked"}"#, GitStatus::Untracked),
        (r#"{"status":"renamed"}"#, GitStatus::Renamed),
        (
            r#"{"status":"modified","staged":true}"#,
            GitStatus::StagedModified,
        ),
        (r#"{"status":"modified"}"#, GitStatus::Modified),
        (r#"{"status":"added","staged":true}"#, GitStatus::Added),
        (r#"{"status":"deleted"}"#, GitStatus::Deleted),
        (r#"{"status":"copied"}"#, GitStatus::Modified),
    ];

    #[test]
    fn the_daemon_word_and_the_staged_flag_pick_the_status() {
        for (text, expected) in CASES {
            let body: FileBody = serde_json::from_str(text).unwrap();
            assert_eq!(body.git_status(), expected, "{text}");
        }
    }
}
