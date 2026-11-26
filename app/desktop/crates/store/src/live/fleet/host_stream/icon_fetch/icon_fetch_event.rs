//! What bringing a space image to disk logs when it cannot be fetched or kept.

use std::fmt;

use journal::Level;
use tokio::task::JoinError;

pub(crate) enum IconFetchEvent<'a> {
    Unfetched {
        error: &'a daemonapi::Error,
        url: &'a str,
    },
    TooLarge {
        url: &'a str,
    },
    Unkept {
        error: &'a JoinError,
    },
}

impl fmt::Display for IconFetchEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unfetched { error, url } => {
                write!(
                    f,
                    "a space image could not be fetched error={error} url={url:?}"
                )
            },
            Self::TooLarge { url } => write!(f, "a space image is larger than allowed url={url:?}"),
            Self::Unkept { error } => write!(f, "keeping a space image failed error={error}"),
        }
    }
}

impl journal::Event for IconFetchEvent<'_> {
    fn source(&self) -> &'static str {
        "store::live::fleet::host_stream::icon_fetch"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unfetched { .. } => "icon_fetch_unfetched",
            Self::TooLarge { .. } => "icon_fetch_too_large",
            Self::Unkept { .. } => "icon_fetch_unkept",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unfetched { .. } | Self::TooLarge { .. } | Self::Unkept { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_image_too_large_warns_naming_its_url() {
        let event = IconFetchEvent::TooLarge {
            url: "/v0/projects/p/icon?v=2",
        };
        let message = "a space image is larger than allowed url=\"/v0/projects/p/icon?v=2\"";
        let seen = (event.name(), event.level(), event.to_string());
        assert_eq!(
            seen,
            ("icon_fetch_too_large", Level::Warn, message.to_owned())
        );
    }
}
