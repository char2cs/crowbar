use keymap::ShellCommand;
use ui::IconName;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NewTabAction {
    Terminal,
    Browser,
    File,
}

impl NewTabAction {
    pub(crate) const ALL: [Self; 3] = [Self::Terminal, Self::Browser, Self::File];

    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::Terminal => "New Terminal",
            Self::Browser => "New Browser",
            Self::File => "New File",
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::Terminal => IconName::TerminalWindow,
            Self::Browser => IconName::Globe,
            Self::File => IconName::FilePlus,
        }
    }

    pub(crate) fn command(self) -> ShellCommand {
        match self {
            Self::Terminal => ShellCommand::NewTerminal,
            Self::Browser => ShellCommand::NewBrowser,
            Self::File => ShellCommand::NewFile,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_page_lists_develops_three_actions_in_order_each_with_its_own_glyph_and_command() {
        let labels: Vec<_> = NewTabAction::ALL.iter().map(|a| a.label()).collect();
        assert_eq!(labels, ["New Terminal", "New Browser", "New File"]);
        let icons: Vec<_> = NewTabAction::ALL.iter().map(|a| a.icon()).collect();
        assert_eq!(
            icons,
            [
                IconName::TerminalWindow,
                IconName::Globe,
                IconName::FilePlus
            ]
        );
        let commands: Vec<_> = NewTabAction::ALL.iter().map(|a| a.command()).collect();
        assert_eq!(
            commands,
            [
                ShellCommand::NewTerminal,
                ShellCommand::NewBrowser,
                ShellCommand::NewFile
            ]
        );
    }
}
