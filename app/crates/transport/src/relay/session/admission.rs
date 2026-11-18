use std::time::Duration;

use protocol::RelayScope;

use crate::relay::connections::Guard;

pub(crate) struct Admission {
    pub(crate) scope: RelayScope,
    pub(crate) path_and_query: String,
    pub(crate) upstream_idle: Option<Duration>,
    pub(crate) guard: Guard,
}
