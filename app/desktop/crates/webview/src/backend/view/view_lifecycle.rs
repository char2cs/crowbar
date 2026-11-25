use crate::error::Result;
use crate::view::Config;
use crate::view::Id;
use crate::view::ParentWindow;

/// Creating and destroying web views.
pub trait ViewLifecycle {
    /// Creates a web view as a child of `parent`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the engine cannot create the view.
    fn create(
        &self,
        id: Id,
        parent: ParentWindow,
        config: Config,
    ) -> Result<()>;

    /// Destroys a web view and frees its resources.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn destroy(
        &self,
        id: Id,
    ) -> Result<()>;

    /// Returns once every request made before it has run, so destroyed views are gone. A backend
    /// that runs each request as it is made has nothing to wait for.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or does not answer in time.
    fn settle(&self) -> Result<()> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct AsAsked;

    impl ViewLifecycle for AsAsked {
        fn create(
            &self,
            _: Id,
            _: ParentWindow,
            _: Config,
        ) -> Result<()> {
            Ok(())
        }

        fn destroy(
            &self,
            _: Id,
        ) -> Result<()> {
            Ok(())
        }
    }

    #[test]
    fn a_backend_that_acts_as_asked_has_settled_at_once() {
        AsAsked.destroy(Id(1)).unwrap();
        assert!(AsAsked.settle().is_ok());
    }
}
