use crate::credential::Credential;
use crate::endpoint::Endpoint;
use crate::http_conn::Channel;

pub(crate) struct Redeemed {
    pub(crate) endpoint: Endpoint,
    pub(crate) credential: Credential,
    pub(crate) client_id: String,
    pub(crate) channel: Channel,
}
