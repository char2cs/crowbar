use http::HeaderValue;

use crate::remote_host::RemoteHost;

#[derive(Debug)]
pub(crate) struct PairedHost {
    pub(crate) host: RemoteHost,
    pub(crate) authorization: HeaderValue,
}
