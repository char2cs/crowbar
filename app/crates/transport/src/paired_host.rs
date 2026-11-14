use http::HeaderValue;
use protocol::HostId;

use crate::remote_host::RemoteHost;

/// A registered host and the credential its daemon expects.
#[derive(Debug)]
pub struct PairedHost {
    /// The host's id.
    pub id: HostId,
    /// Where it listens and the certificate trusted there.
    pub host: RemoteHost,
    /// The `Authorization` header value to send.
    pub authorization: HeaderValue,
}
