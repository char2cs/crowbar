/// Every way a connection can fail.
#[derive(Debug)]
pub enum Error {
    /// The host refused the token.
    Refused,
}
