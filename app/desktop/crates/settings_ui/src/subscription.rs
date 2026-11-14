//! A registration that ends when it is dropped.

/// Keeps a listener registered with a backend; dropping it unregisters the listener.
pub struct Subscription {
    cancel: Option<Box<dyn FnOnce()>>,
}

impl Subscription {
    /// A subscription that runs `cancel` when dropped.
    pub fn new(cancel: impl FnOnce() + 'static) -> Self {
        Self {
            cancel: Some(Box::new(cancel)),
        }
    }
}

impl Drop for Subscription {
    fn drop(&mut self) {
        if let Some(cancel) = self.cancel.take() {
            cancel();
        }
    }
}

impl std::fmt::Debug for Subscription {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("Subscription")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
#[path = "subscription_tests.rs"]
mod tests;
