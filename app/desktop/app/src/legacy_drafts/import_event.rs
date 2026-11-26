//! What importing the previous app's drafts and settings tells the log.

use std::fmt;

use journal::Level;

pub(crate) enum ImportEvent<'a> {
    DraftsNotImported { error: &'a store::Error },
    DraftsNotWritten { error: &'a std::io::Error },
    DraftsImported { kept: usize },
    SettingsNotImported { error: &'a store::Error },
    SettingsImported { taken: usize },
}

impl fmt::Display for ImportEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::DraftsNotImported { error } => {
                write!(
                    f,
                    "drafts of the previous editor were not imported error={error}"
                )
            },
            Self::DraftsNotWritten { error } => {
                write!(f, "imported drafts were not written error={error}")
            },
            Self::DraftsImported { kept } => {
                write!(
                    f,
                    "unsaved drafts of the previous editor were imported kept={kept}"
                )
            },
            Self::SettingsNotImported { error } => {
                write!(
                    f,
                    "settings of the previous app were not imported error={error}"
                )
            },
            Self::SettingsImported { taken } => {
                write!(
                    f,
                    "settings of the previous app were imported taken={taken}"
                )
            },
        }
    }
}

impl journal::Event for ImportEvent<'_> {
    fn source(&self) -> &'static str {
        match self {
            Self::DraftsNotImported { .. }
            | Self::DraftsNotWritten { .. }
            | Self::DraftsImported { .. } => "crowbar::legacy_drafts",
            Self::SettingsNotImported { .. } | Self::SettingsImported { .. } => {
                "crowbar::legacy_settings"
            },
        }
    }

    fn name(&self) -> &'static str {
        match self {
            Self::DraftsNotImported { .. } => "legacy_drafts_not_imported",
            Self::DraftsNotWritten { .. } => "legacy_drafts_not_written",
            Self::DraftsImported { .. } => "legacy_drafts_imported",
            Self::SettingsNotImported { .. } => "legacy_settings_not_imported",
            Self::SettingsImported { .. } => "legacy_settings_imported",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::DraftsImported { .. } | Self::SettingsImported { .. } => Level::Info,
            Self::DraftsNotImported { .. }
            | Self::DraftsNotWritten { .. }
            | Self::SettingsNotImported { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_drafts_import_event_names_itself_and_reads_as_the_old_log_line() {
        let unread = store::Error::Stream("gone".into());
        let unwritten = std::io::Error::other("disk full");
        let cases = [
            (
                ImportEvent::DraftsNotImported { error: &unread },
                "legacy_drafts_not_imported",
                Level::Warn,
                "drafts of the previous editor were not imported error=shell data: stream: gone",
            ),
            (
                ImportEvent::DraftsNotWritten { error: &unwritten },
                "legacy_drafts_not_written",
                Level::Warn,
                "imported drafts were not written error=disk full",
            ),
            (
                ImportEvent::DraftsImported { kept: 2 },
                "legacy_drafts_imported",
                Level::Info,
                "unsaved drafts of the previous editor were imported kept=2",
            ),
        ];
        for (event, name, level, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }

    #[test]
    fn each_settings_import_event_names_itself_and_reads_as_the_old_log_line() {
        let unread = store::Error::Stream("gone".into());
        let cases = [
            (
                ImportEvent::SettingsNotImported { error: &unread },
                "legacy_settings_not_imported",
                Level::Warn,
                "settings of the previous app were not imported error=shell data: stream: gone",
            ),
            (
                ImportEvent::SettingsImported { taken: 3 },
                "legacy_settings_imported",
                Level::Info,
                "settings of the previous app were imported taken=3",
            ),
        ];
        for (event, name, level, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
