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

    /// The pixels of several views at once. A backend that has to ask each view for its pixels
    /// asks them together and waits once, so a slow view costs one deadline, not one each.
    /// The answers are in the order of `ids`.
    fn snapshot_all(
        &self,
        ids: &[Id],
    ) -> Vec<Result<Option<Snapshot>>> {
        ids.iter().map(|id| self.snapshot(*id)).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Blind;

    impl ViewCapture for Blind {}

    #[test]
    fn a_backend_that_cannot_read_pixels_answers_none_for_every_view() {
        let answers = Blind.snapshot_all(&[Id(1), Id(2)]);
        assert_eq!(answers.len(), 2, "one answer per id");
        assert!(
            answers.iter().all(|answer| matches!(answer, Ok(None))),
            "no pixels"
        );
    }
}
