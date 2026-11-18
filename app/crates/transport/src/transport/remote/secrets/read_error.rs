/// A failed raw keychain read. `denied` means the user refused access, which is remembered for
/// the session; anything else may succeed on a later read.
#[derive(Debug, Clone)]
pub struct ReadError {
    /// What the keychain said.
    pub message: String,
    /// Whether the failure is a refusal that must not be re-prompted.
    pub denied: bool,
}
