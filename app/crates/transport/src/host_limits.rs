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
    pub(crate) async fn acquire(
        &self,
        host: &HostId,
    ) -> Option<OwnedSemaphorePermit> {
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
mod tests {
    use std::time::Duration;

    use super::*;

    #[tokio::test(start_paused = true)]
    async fn one_full_host_makes_its_own_requests_wait_but_not_another_hosts() {
        let limits = HostLimits::default();
        let busy = HostId::new("host-1").unwrap();
        let other = HostId::new("host-2").unwrap();
        let mut held = Vec::new();
        for _ in 0..REQUESTS_PER_HOST {
            held.push(limits.acquire(&busy).await.unwrap());
        }
        let waited = tokio::time::timeout(Duration::from_secs(1), limits.acquire(&busy)).await;
        assert!(waited.is_err(), "the 33rd request for a busy host queues");
        assert!(limits.acquire(&other).await.is_some());
        held.pop();
        assert!(
            limits.acquire(&busy).await.is_some(),
            "a finished request frees a slot"
        );
    }
}
