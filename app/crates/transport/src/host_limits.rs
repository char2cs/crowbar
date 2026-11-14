use std::collections::HashMap;
use std::sync::{Arc, Mutex, PoisonError};

use protocol::HostId;
use tokio::sync::{OwnedSemaphorePermit, Semaphore};

pub(crate) const REQUESTS_PER_HOST: usize = 32;

#[derive(Debug, Default)]
pub(crate) struct HostLimits {
    hosts: Mutex<HashMap<HostId, Arc<Semaphore>>>,
}

impl HostLimits {
    pub(crate) async fn acquire(&self, host: &HostId) -> Option<OwnedSemaphorePermit> {
        let semaphore = {
            let mut hosts = self.hosts.lock().unwrap_or_else(PoisonError::into_inner);
            Arc::clone(
                hosts
                    .entry(host.clone())
                    .or_insert_with(|| Arc::new(Semaphore::new(REQUESTS_PER_HOST))),
            )
        };
        semaphore.acquire_owned().await.ok()
    }
}

#[cfg(test)]
#[path = "host_limits_tests.rs"]
mod tests;
