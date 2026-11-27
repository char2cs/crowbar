#[macro_export]
macro_rules! sink {
    ($target:literal) => {
        pub(crate) fn emit(event: &dyn $crate::Event) {
            match event.level() {
                $crate::Level::Error => emit_error(event),
                $crate::Level::Warn => emit_warn(event),
                $crate::Level::Info => emit_info(event),
                $crate::Level::Debug => emit_debug(event),
                $crate::Level::Trace => emit_trace(event),
            }
        }

        fn emit_error(event: &dyn $crate::Event) {
            $crate::tracing::error!(target: $target, message = %event, source = event.source(), event = event.name());
        }

        fn emit_warn(event: &dyn $crate::Event) {
            $crate::tracing::warn!(target: $target, message = %event, source = event.source(), event = event.name());
        }

        fn emit_info(event: &dyn $crate::Event) {
            $crate::tracing::info!(target: $target, message = %event, source = event.source(), event = event.name());
        }

        fn emit_debug(event: &dyn $crate::Event) {
            $crate::tracing::debug!(target: $target, message = %event, source = event.source(), event = event.name());
        }

        fn emit_trace(event: &dyn $crate::Event) {
            $crate::tracing::trace!(target: $target, message = %event, source = event.source(), event = event.name());
        }
    };
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex, PoisonError};
    use std::{fmt, io};

    use crate::{Event, Level};

    crate::sink!("journaltest");

    struct Said(Level);

    impl fmt::Display for Said {
        fn fmt(
            &self,
            f: &mut fmt::Formatter<'_>,
        ) -> fmt::Result {
            write!(f, "said at {:?}", self.0)
        }
    }

    impl Event for Said {
        fn source(&self) -> &'static str {
            "journaltest::module"
        }

        fn name(&self) -> &'static str {
            "said"
        }

        fn level(&self) -> Level {
            self.0
        }
    }

    #[derive(Clone, Default)]
    struct Captured(Arc<Mutex<Vec<u8>>>);

    impl io::Write for Captured {
        fn write(
            &mut self,
            bytes: &[u8],
        ) -> io::Result<usize> {
            let mut held = self.0.lock().unwrap_or_else(PoisonError::into_inner);
            held.extend_from_slice(bytes);
            Ok(bytes.len())
        }

        fn flush(&mut self) -> io::Result<()> {
            Ok(())
        }
    }

    fn logged(event: &Said) -> String {
        let captured = Captured::default();
        let writer = captured.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_writer(move || writer.clone())
            .with_max_level(tracing::Level::TRACE)
            .without_time()
            .finish();
        tracing::subscriber::with_default(subscriber, || emit(event));
        let bytes = captured
            .0
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone();
        String::from_utf8_lossy(&bytes).into_owned()
    }

    #[test]
    fn an_event_logs_at_its_level_under_the_crate_target_with_its_name_and_source() {
        let cases = [
            (Level::Error, "ERROR"),
            (Level::Warn, " WARN"),
            (Level::Info, " INFO"),
            (Level::Debug, "DEBUG"),
            (Level::Trace, "TRACE"),
        ];
        for (level, shown) in cases {
            let line = logged(&Said(level));
            let expected = format!(
                "{shown} journaltest: said at {level:?} source=\"journaltest::module\" event=\"said\""
            );
            assert_eq!(line.trim(), expected.trim(), "{level:?}");
        }
    }
}
