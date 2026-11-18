/// Whether a request took up the wait for the next page load.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Arming {
    /// The request now waits for the load that follows.
    Armed,
    /// Nobody waits for the request any more, so there is nothing to arm.
    NotWaiting,
}
