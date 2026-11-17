use std::time::Instant;

pub(crate) trait Clock {
    fn now(&self) -> Instant;
}

impl<F: Fn() -> Instant> Clock for F {
    fn now(&self) -> Instant {
        self()
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;
    use std::time::Duration;

    use super::*;

    #[test]
    fn a_closure_is_a_clock_that_reads_what_it_returns() {
        let base = Instant::now();
        let reading = Rc::new(Cell::new(base));
        let seen = Rc::clone(&reading);
        let clock: Rc<dyn Clock> = Rc::new(move || seen.get());
        assert_eq!(clock.now(), base, "starts at the base");
        reading.set(base + Duration::from_millis(5));
        assert_eq!(
            clock.now(),
            base + Duration::from_millis(5),
            "follows the cell"
        );
    }
}
