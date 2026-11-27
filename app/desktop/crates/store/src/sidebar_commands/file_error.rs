use std::path::PathBuf;

use crate::error::Error;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum FileError {
    #[error("{}", .0.reason())]
    Daemon(#[from] Error),
    #[error("{}: {source}", .path.display())]
    Local {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("{} has no file name", .0.display())]
    NoFileName(PathBuf),
    #[error("reading the file was interrupted: {0}")]
    Interrupted(#[from] tokio::task::JoinError),
    #[error("{0} is the file the template is made from")]
    TemplateOfItself(String),
}

impl FileError {
    #[must_use]
    pub fn refusal(&self) -> (u16, Option<&str>) {
        match self {
            Self::Daemon(Error::Daemon(daemonapi::Error::Refused { status, code, .. })) => {
                (*status, code.as_deref())
            },
            _ => (0, None),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_failure_reads_in_words_fit_to_show() {
        let refused = FileError::Daemon(Error::Daemon(daemonapi::Error::Refused {
            path: "/v0/x".into(),
            status: 409,
            code: None,
            message: "name taken".into(),
            data: Box::default(),
        }));
        let missing = FileError::Local {
            path: "/a/b".into(),
            source: std::io::ErrorKind::NotFound.into(),
        };
        assert_eq!(refused.to_string(), "name taken");
        assert_eq!(refused.refusal(), (409, None), "refused by the daemon");
        assert_eq!(missing.refusal(), (0, None), "not the daemon's");
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
