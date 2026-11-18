use tokio::time::Instant;

pub(crate) fn tokio_now() -> Instant {
    Instant::now()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_clock_never_runs_backwards() {
        let earlier = tokio_now();
        assert!(tokio_now() >= earlier);
    }
}
