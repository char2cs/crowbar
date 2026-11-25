use std::fmt::Display;

journal::sink!("crowbar");

pub(crate) fn logged<T, E: Display>(
    result: Result<T, E>,
    what: &str,
) -> Option<T> {
    match result {
        Ok(value) => Some(value),
        Err(error) => {
            tracing::error!(%error, "{what}");
            None
        },
    }
}

pub(crate) fn closed_view<T, E>(
    delivered: &Result<T, E>,
    what: &str,
) {
    if delivered.is_err() {
        tracing::debug!(what, "the view closed before it could be told");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_failure_is_logged_and_becomes_nothing_while_a_value_passes() {
        assert_eq!(logged(Ok::<_, String>(3), "x"), Some(3), "value");
        assert_eq!(logged(Err::<u8, _>("no"), "x"), None, "failure");
    }

    #[test]
    fn telling_a_closed_view_is_only_noted() {
        closed_view(&Err::<(), _>("closed"), "x");
        closed_view(&Ok::<_, String>(()), "x");
    }
}
