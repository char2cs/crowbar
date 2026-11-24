use ui::{ButtonVariant, IconName};

/// What the user can ask of the shell from the halt gate.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Command {
    /// Show the daemon log.
    OpenLog,
    /// Quit Crowbar.
    Quit,
}

impl Command {
    /// The button label.
    pub fn label(self) -> &'static str {
        match self {
            Self::OpenLog => "Open log",
            Self::Quit => "Quit Crowbar",
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::OpenLog => IconName::TerminalWindow,
            Self::Quit => IconName::X,
        }
    }

    pub(crate) fn variant(
        self,
        primary: Self,
    ) -> ButtonVariant {
        if self == primary {
            return ButtonVariant::Primary;
        }
        ButtonVariant::Outline
    }

    pub(crate) fn id(self) -> &'static str {
        match self {
            Self::OpenLog => "startup-open-log",
            Self::Quit => "startup-quit",
        }
    }
}

#[cfg(test)]
mod tests {
    use ui::{ButtonVariant, IconName};

    use super::*;

    const ALL: [Command; 2] = [Command::OpenLog, Command::Quit];

    #[test]
    fn every_command_has_a_distinct_label_and_id() {
        let labels: Vec<_> = ALL.iter().map(|command| command.label()).collect();
        let ids: Vec<_> = ALL.iter().map(|command| command.id()).collect();
        for list in [labels, ids] {
            let mut sorted = list.clone();
            sorted.sort_unstable();
            sorted.dedup();
            assert_eq!(sorted.len(), list.len(), "{list:?}");
        }
    }

    #[test]
    fn only_the_primary_command_is_filled() {
        assert_eq!(Command::Quit.variant(Command::Quit), ButtonVariant::Primary);
        assert_eq!(
            Command::OpenLog.variant(Command::Quit),
            ButtonVariant::Outline
        );
        assert_eq!(Command::Quit.icon(), IconName::X);
    }
}
