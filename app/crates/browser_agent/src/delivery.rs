/// What became of an answer offered to [`Replies`](crate::Replies).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Delivery {
    /// The request was waiting and has its answer.
    Delivered,
    /// Nobody waits for the request any more: it ended, was forgotten or never existed.
    NotWaiting,
    /// The request waits for someone else, so the answer was refused and the wait goes on.
    Refused,
}
