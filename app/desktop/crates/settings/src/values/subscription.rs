pub struct Subscription {
    cancel: Option<Box<dyn FnOnce()>>,
}

impl Subscription {
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
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Subscription")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use super::*;

    #[test]
    fn dropping_cancels_exactly_once() {
        let cancelled = Rc::new(Cell::new(0));
        let sink = Rc::clone(&cancelled);
        let subscription = Subscription::new(move || sink.set(sink.get() + 1));
        assert_eq!(cancelled.get(), 0, "alive subscriptions stay registered");
        drop(subscription);
        assert_eq!(cancelled.get(), 1, "drop cancels once");
    }
}
