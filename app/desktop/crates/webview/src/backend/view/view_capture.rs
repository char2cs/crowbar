use crate::error::Result;
use crate::view::Id;
use crate::view::Snapshot;

/// Reading back the pixels of web views.
pub trait ViewCapture {
    /// The pixels the web view shows now, or `None` when this backend cannot read them back or
    /// the page did not answer in time.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn snapshot(
        &self,
        _id: Id,
    ) -> Result<Option<Snapshot>> {
        Ok(None)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Blind;

    impl ViewCapture for Blind {}

    #[test]
    fn a_backend_that_cannot_read_pixels_answers_none() {
        assert!(matches!(Blind.snapshot(Id(1)), Ok(None)), "no pixels");
    }
}
