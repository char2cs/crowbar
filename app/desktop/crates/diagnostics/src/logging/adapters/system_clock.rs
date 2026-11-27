use std::time::SystemTime;

pub(crate) fn now() -> SystemTime {
    SystemTime::now()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_clock_reads_a_time_after_the_epoch() {
        assert!(now() > SystemTime::UNIX_EPOCH);
    }
}
