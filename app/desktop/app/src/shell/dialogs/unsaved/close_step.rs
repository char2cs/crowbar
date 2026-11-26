use crate::shell::dialogs::unsaved::closing::Closing;

#[derive(Debug, PartialEq)]
pub(crate) enum CloseStep<Pending> {
    Run(Closing),
    Wait(Pending),
    Stop,
}
