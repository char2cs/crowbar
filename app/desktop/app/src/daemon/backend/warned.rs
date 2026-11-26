#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Warned {
    #[default]
    No,
    Yes,
}

/// Whether the backend has answered since launch; only a drop after that is a "reconnect".
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Reach {
    #[default]
    Launching,
    Reached,
}
