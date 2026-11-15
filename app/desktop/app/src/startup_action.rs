#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum StartupAction {
    Relaunch,
    ShowLog,
    QuitApp,
    Nothing,
}
