use http::HeaderValue;

use crate::transport::remote::host::Host;

#[derive(Debug)]
pub(crate) struct PairedHost {
    pub(crate) host: Host,
    pub(crate) authorization: HeaderValue,
}
