//! How one stream connection finished.

pub(crate) enum Ended {
    /// The socket went away; dial again.
    Dropped,
    /// The shell stopped asking for the stream.
    Stopped,
}
