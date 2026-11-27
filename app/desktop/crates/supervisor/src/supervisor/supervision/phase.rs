pub(crate) enum Phase {
    Launch,
    Recover { reason: String },
    Idle,
    Stop,
}
