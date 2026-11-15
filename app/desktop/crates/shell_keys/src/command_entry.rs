use crate::shell_command::ShellCommand;

pub(crate) struct CommandEntry {
    pub(crate) command: ShellCommand,
    pub(crate) id: &'static str,
    pub(crate) label: &'static str,
    pub(crate) chord: &'static str,
}
