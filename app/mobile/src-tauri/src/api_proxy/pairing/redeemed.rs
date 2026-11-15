use crate::api_proxy::channel::Channel;
use crate::host_tokens::Credential;

pub(super) struct Redeemed {
    pub(super) credential: Credential,
    pub(super) client_id: String,
    pub(super) channel: Channel,
}
