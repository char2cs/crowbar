use super::read_error::ReadError;
use crate::token_error::TokenError;

pub(super) trait Keychain: Send + Sync {
    fn read(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, ReadError>;
    fn write(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), TokenError>;
    fn remove(
        &self,
        host_id: &str,
    ) -> Result<(), TokenError>;
}
