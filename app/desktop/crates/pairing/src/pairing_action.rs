#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum PairingAction {
    Edited,
    SubmitAddress { name: String, address: String },
    Confirm,
    Back,
    SubmitCode(String),
}
