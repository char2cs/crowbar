#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Action {
    Edited,
    SubmitAddress { name: String, address: String },
    Confirm,
    Back,
    SubmitCode(String),
}
