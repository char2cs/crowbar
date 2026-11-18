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
}
