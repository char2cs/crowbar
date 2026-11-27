use crate::error::Result;
use crate::view::Config;
use crate::view::Id;
use crate::view::ParentWindow;

pub trait ViewLifecycle {
    fn create(
        &self,
        id: Id,
        parent: ParentWindow,
        config: Config,
    ) -> Result<()>;

    fn destroy(
        &self,
        id: Id,
    ) -> Result<()>;

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
