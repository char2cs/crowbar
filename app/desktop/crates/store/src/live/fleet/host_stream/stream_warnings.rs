//! The warnings a host's stream logs when its dial fails or the daemon ends or refuses a request.

use daemonapi::Problem;

pub(crate) fn unopened(
    host: &str,
    error: &daemonapi::Error,
) {
    tracing::warn!(host = %host, %error, "the stream could not be opened");
}

pub(crate) fn closed(
    host: &str,
    scope: &str,
    reason: &str,
    problem: Option<&Problem>,
) {
    tracing::warn!(host = %host, scope, reason, ?problem, "the daemon ended a stream scope");
}

pub(crate) fn refused(
    host: &str,
    problem: &Problem,
) {
    tracing::warn!(host = %host, ?problem, "the daemon refused a stream request");
}
