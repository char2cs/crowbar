use std::time::Duration;

/// The time source of the view: a monotonic duration since any fixed origin.
pub trait StartupClock {
    /// The time now.
    fn now(&self) -> Duration;
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;
    use std::time::Duration;

    use super::*;
    use crate::test_support::ManualClock;

    #[test]
    fn a_clock_is_usable_as_a_shared_trait_object() {
        let manual = Rc::new(ManualClock::default());
        let clock = Rc::clone(&manual) as Rc<dyn StartupClock>;
        manual.set(Duration::from_secs(3));
        assert_eq!(clock.now(), Duration::from_secs(3));
    }
}
