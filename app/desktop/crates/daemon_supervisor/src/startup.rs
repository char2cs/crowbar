pub(crate) enum Startup {
    Ready(Option<u32>),
    Halted(String),
    NeverHealthy,
    Exited(String),
    Stopped,
}
