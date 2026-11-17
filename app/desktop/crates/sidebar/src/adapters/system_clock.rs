use std::time::Instant;

use crate::clock::Clock;

pub(crate) fn system_clock() -> impl Clock {
    Instant::now
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn time_never_goes_backwards() {
        let clock = system_clock();
        let first = clock.now();
        assert!(clock.now() >= first);
    }
}
