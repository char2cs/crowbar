//! Why an action on the workspace's files failed.

use std::path::PathBuf;

use crate::error::Error;

/// Why an action on the workspace's files failed; its text is fit to show.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum FileError {
    /// The daemon refused or could not be asked.
    #[error("{}", .0.reason())]
    Daemon(#[from] Error),
    /// A local file could not be read.
    #[error("{}: {source}", .path.display())]
    Local {
        /// The file.
        path: PathBuf,
        /// The cause.
        #[source]
        source: std::io::Error,
    },
    /// A local path has no file name to copy under.
    #[error("{} has no file name", .0.display())]
    NoFileName(PathBuf),
    /// The reading task ended before it answered.
    #[error("reading the file was interrupted: {0}")]
    Interrupted(#[from] tokio::task::JoinError),
    /// The template would overwrite the file it is made from.
    #[error("{0} is the file the template is made from")]
    TemplateOfItself(String),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_failure_reads_in_words_fit_to_show() {
        let refused = FileError::Daemon(Error::Refused {
            path: "/v0/x".into(),
            status: 409,
            code: None,
            message: "name taken".into(),
        });
        let missing = FileError::Local {
            path: "/a/b".into(),
            source: std::io::ErrorKind::NotFound.into(),
        };
        assert_eq!(refused.to_string(), "name taken");
        assert!(missing.to_string().starts_with("/a/b: "));
        assert_eq!(
            FileError::NoFileName("/".into()).to_string(),
            "/ has no file name"
        );
        assert_eq!(
            FileError::TemplateOfItself(".env".into()).to_string(),
            ".env is the file the template is made from"
        );
    }
}
