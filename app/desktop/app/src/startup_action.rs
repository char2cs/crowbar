#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum StartupAction {
    ShowLog,
    QuitApp,
    Nothing,
}
