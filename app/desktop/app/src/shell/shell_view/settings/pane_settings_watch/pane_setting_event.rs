use std::fmt;

use journal::Level;

pub(super) enum PaneSettingEvent<'a> {
    UnknownShape {
        key: &'a protocol::SettingKey,
    },
    Unstored {
        key: &'a protocol::SettingKey,
        error: &'a settings::Error,
    },
}

impl fmt::Display for PaneSettingEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::UnknownShape { key } => {
                write!(
                    f,
                    "a pane wrote a setting value of no known shape key={key}"
                )
            },
            Self::Unstored { key, error } => {
                write!(f, "storing a pane's setting failed key={key} error={error}")
            },
        }
    }
}

impl journal::Event for PaneSettingEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_view::pane_settings_watch"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::UnknownShape { .. } => "pane_setting_unknown_shape",
            Self::Unstored { .. } => "pane_setting_unstored",
        }
    }

    fn level(&self) -> Level {
        Level::Warn
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_pane_setting_event_names_itself_and_reads_as_the_old_log_line() {
        let key = protocol::SettingKey::new("w").unwrap();
        let refused = settings::Error::Unavailable("x".into());
        let cases = [
            (
                PaneSettingEvent::UnknownShape { key: &key },
                "pane_setting_unknown_shape",
                "a pane wrote a setting value of no known shape key=w",
            ),
            (
                PaneSettingEvent::Unstored {
                    key: &key,
                    error: &refused,
                },
                "pane_setting_unstored",
                "storing a pane's setting failed key=w error=settings backend is unavailable: x",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
