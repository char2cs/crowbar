use crate::hosts::Endpoint;
use crate::transport::Channel;
use crate::transport::Credential;

pub(crate) struct Redeemed {
    pub(crate) endpoint: Endpoint,
    pub(crate) credential: Credential,
    pub(crate) client_id: String,
    pub(crate) channel: Channel,
}
