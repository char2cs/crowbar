use crate::values::error::Error;
use crate::values::key::Key;
use crate::values::value::Value;

pub trait Write {
    fn set(
        &self,
        key: &Key,
        value: Value,
    ) -> Result<(), Error>;

    fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error>;
}
