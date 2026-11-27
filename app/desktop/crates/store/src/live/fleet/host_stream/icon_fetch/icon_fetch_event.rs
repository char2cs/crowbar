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

    const URL: &str = "/v0/projects/p/icon?v=2";

    #[tokio::test]
    async fn each_event_has_its_name_level_and_message() {
        let error = daemonapi::Error::Stream("reset".into());
        let task = tokio::spawn(std::future::pending::<()>());
        task.abort();
        let cancelled = task
            .await
            .expect_err("an aborted task ends with a join error");
        let cases = [
            (
                IconFetchEvent::Unfetched {
                    error: &error,
                    url: URL,
                },
                "icon_fetch_unfetched",
                format!("a space image could not be fetched error={error} url=\"{URL}\""),
            ),
            (
                IconFetchEvent::TooLarge { url: URL },
                "icon_fetch_too_large",
                format!("a space image is larger than allowed url=\"{URL}\""),
            ),
            (
                IconFetchEvent::Unkept { error: &cancelled },
                "icon_fetch_unkept",
                format!("keeping a space image failed error={cancelled}"),
            ),
        ];
        assert_each(cases, Level::Warn);
    }

    fn assert_each<M: fmt::Display>(
        cases: impl IntoIterator<Item = (impl Event, &'static str, M)>,
        level: Level,
    ) {
        for (event, name, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown.to_string(), "{name}");
        }
    }
}
