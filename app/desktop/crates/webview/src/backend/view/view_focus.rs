use crate::error::Result;
use crate::view::Id;

pub trait ViewFocus {
    fn focus(
        &self,
        id: Id,
    ) -> Result<()>;

    fn focus_parent(
        &self,
        id: Id,
    ) -> Result<()>;
}
