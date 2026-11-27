use crate::error::Result;
use crate::view::Id;

pub trait PageControl {
    fn eval(
        &self,
        id: Id,
        script: &str,
    ) -> Result<()>;

    fn navigate(
        &self,
        id: Id,
        url: &str,
    ) -> Result<()>;
}
