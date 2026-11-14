use std::sync::{Arc, Mutex};

use tokio_util::sync::CancellationToken;

use crate::connection_book::ConnectionBook;
use crate::origin_policy::OriginPolicy;
use crate::relay_config::RelayConfig;
use crate::token_book::TokenBook;
use crate::transport::Transport;

pub(crate) struct RelayState {
    pub(crate) transport: Arc<Transport>,
    pub(crate) config: RelayConfig,
    pub(crate) policy: Arc<dyn OriginPolicy>,
    pub(crate) tokens: Mutex<TokenBook>,
    pub(crate) connections: Arc<ConnectionBook>,
    pub(crate) port: u16,
    pub(crate) shutdown: CancellationToken,
}
