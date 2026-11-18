pub(crate) enum Startup {
    Ready(Option<u32>),
    Halted {
        reason: Option<String>,
        message: String,
    },
    NeverHealthy,
    Exited(String),
    Stopped,
}
