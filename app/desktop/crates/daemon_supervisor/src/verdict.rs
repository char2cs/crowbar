#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Verdict {
    Respawn { attempt: usize },
    GiveUp { deaths: usize },
}
