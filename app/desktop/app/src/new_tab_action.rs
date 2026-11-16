use shell_keys::ShellCommand;
use ui::IconName;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NewTabAction {
    Terminal,
    Browser,
    File,
    Review,
}

impl NewTabAction {
    pub(crate) const ALL: [Self; 4] = [Self::Terminal, Self::Browser, Self::File, Self::Review];

    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::Terminal => "New Terminal",
            Self::Browser => "New Browser",
            Self::File => "New File",
            Self::Review => "Review Branch",
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::Terminal => IconName::Terminal,
            Self::Browser => IconName::Globe,
            Self::File => IconName::FilePlus,
            Self::Review => IconName::GitPullRequest,
        }
    }

    pub(crate) fn command(self) -> ShellCommand {
        match self {
            Self::Terminal => ShellCommand::NewTerminal,
            Self::Browser => ShellCommand::NewBrowser,
            Self::File => ShellCommand::NewFile,
            Self::Review => ShellCommand::OpenReview,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_page_lists_the_four_actions_in_order_each_with_its_own_command() {
        let labels: Vec<_> = NewTabAction::ALL.iter().map(|a| a.label()).collect();
        assert_eq!(
            labels,
            ["New Terminal", "New Browser", "New File", "Review Branch"]
        );
        let commands: Vec<_> = NewTabAction::ALL.iter().map(|a| a.command()).collect();
        assert_eq!(
            commands,
            [
                ShellCommand::NewTerminal,
                ShellCommand::NewBrowser,
                ShellCommand::NewFile,
                ShellCommand::OpenReview
            ]
        );
    }
}
