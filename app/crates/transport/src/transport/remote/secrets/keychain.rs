use crate::error::Error;
use crate::transport::remote::secrets::read_error::ReadError;

pub trait Keychain: Send + Sync {
    fn read(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, ReadError>;

    fn write(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error>;

    fn remove(
        &self,
        host_id: &str,
    ) -> Result<(), Error>;
}
