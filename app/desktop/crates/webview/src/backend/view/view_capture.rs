use crate::error::Result;
use crate::view::Id;
use crate::view::Snapshot;

pub trait ViewCapture {
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
