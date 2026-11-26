//! What keeping a terminal's zoom as a setting tells the log.

use std::fmt;

use journal::Level;

pub(super) enum ZoomEvent<'a> {
    SettingsClosed,
    Unkept { error: &'a settings::Error },
}

impl fmt::Display for ZoomEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::SettingsClosed => {
                write!(f, "the settings are not open; a terminal zoom is not kept")
            },
            Self::Unkept { error } => write!(f, "keeping the terminal zoom failed error={error}"),
        }
    }
}

impl journal::Event for ZoomEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_view::terminal_zoom"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::SettingsClosed => "terminal_zoom_settings_closed",
            Self::Unkept { .. } => "terminal_zoom_unkept",
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
    fn each_zoom_event_names_itself_and_reads_as_the_old_log_line() {
        let refused = settings::Error::Unavailable("closed".into());
        let cases = [
            (
                ZoomEvent::SettingsClosed,
                "terminal_zoom_settings_closed",
                "the settings are not open; a terminal zoom is not kept",
            ),
            (
                ZoomEvent::Unkept { error: &refused },
                "terminal_zoom_unkept",
                "keeping the terminal zoom failed error=settings backend is unavailable: closed",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
