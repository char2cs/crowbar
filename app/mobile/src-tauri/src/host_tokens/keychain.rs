use super::read_error::ReadError;

pub(super) trait Keychain: Send + Sync {
    fn read(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, ReadError>;
    fn write(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), String>;
    fn remove(
        &self,
        host_id: &str,
    ) -> Result<(), String>;
}
