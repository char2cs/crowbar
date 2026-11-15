use std::time::Duration;

pub(super) struct OpenRequest {
    pub(super) conn_id: String,
    pub(super) path: String,
    pub(super) host_id: Option<String>,
    pub(super) window_label: String,
    pub(super) read_idle_timeout: Option<Duration>,
}
