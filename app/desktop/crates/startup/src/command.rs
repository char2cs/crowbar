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
    pub(crate) fn label(self) -> &'static str {
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
    use super::*;

    #[test]
    fn only_the_primary_command_is_filled_and_each_has_its_own_label_and_id() {
        let (log, quit) = (Command::OpenLog, Command::Quit);
        assert_eq!(
            (quit.variant(quit), log.variant(quit)),
            (ButtonVariant::Primary, ButtonVariant::Outline)
        );
        assert!(log.label() != quit.label() && log.id() != quit.id());
    }
}
