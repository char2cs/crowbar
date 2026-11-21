const FORBIDDEN: u16 = 403;
const NOT_FOUND: u16 = 404;
const TOO_LARGE: u16 = 413;
const HOST_DOCUMENT_TRANSPORT: &str = "host_document_requires_local_transport";
const DAEMON_STATE_DOCUMENT: &str = "daemon_state_document";

/// Why a document could not be read, in develop's words (`openFailure` in `open-document.ts`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OpenFailure {
    title: String,
    message: String,
}

impl OpenFailure {
    /// The failure the daemon's answer `status` (0 when it could not be asked) and problem `code`
    /// mean for the document at `path`.
    #[must_use]
    pub fn of(
        path: &str,
        status: u16,
        code: Option<&str>,
    ) -> Self {
        let name = path.rsplit('/').next().unwrap_or(path).to_owned();
        let (title, message) = match (status, code) {
            (FORBIDDEN, Some(HOST_DOCUMENT_TRANSPORT)) => (
                "Can't open this file here",
                "Files outside your workspaces open only in the Crowbar desktop app.".to_owned(),
            ),
            (FORBIDDEN, Some(DAEMON_STATE_DOCUMENT)) => (
                "Can't open this file",
                "It holds Crowbar's own state.".to_owned(),
            ),
            (FORBIDDEN, _) => ("Can't open file: permission denied", name),
            (NOT_FOUND, _) => ("File not found", path.to_owned()),
            (TOO_LARGE, _) => ("File too large to open", name),
            _ => ("Failed to open file", name),
        };
        Self {
            title: title.to_owned(),
            message,
        }
    }

    /// The headline: "File not found".
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    /// The line under it: the file's path or name, or the reason.
    #[must_use]
    pub fn message(&self) -> &str {
        &self.message
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (u16, Option<&'static str>, &'static str, &'static str);

    const CASES: [Case; 6] = [
        (NOT_FOUND, None, "File not found", "src/a.ts"),
        (TOO_LARGE, None, "File too large to open", "a.ts"),
        (
            FORBIDDEN,
            None,
            "Can't open file: permission denied",
            "a.ts",
        ),
        (
            FORBIDDEN,
            Some(DAEMON_STATE_DOCUMENT),
            "Can't open this file",
            "It holds Crowbar's own state.",
        ),
        (500, None, "Failed to open file", "a.ts"),
        (0, None, "Failed to open file", "a.ts"),
    ];

    #[test]
    fn each_answer_reads_as_develop_says_it() {
        for (status, code, title, message) in CASES {
            let failure = OpenFailure::of("src/a.ts", status, code);
            assert_eq!(
                (failure.title(), failure.message()),
                (title, message),
                "{status}"
            );
        }
    }
}
