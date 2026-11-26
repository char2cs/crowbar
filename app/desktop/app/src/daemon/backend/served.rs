/// Whether the backend has answered since the shell started: only then is a lost link an outage.
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Served {
    #[default]
    Never,
    Before,
}
