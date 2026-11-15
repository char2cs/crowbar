const LOCAL_HOST: &str = "localhost";

pub(crate) fn is_remote_host(host: &str) -> bool {
    !host.is_empty() && host != LOCAL_HOST
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_named_host_other_than_the_local_daemon_is_remote() {
        const CASES: &[(&str, bool)] = &[
            ("", false),
            ("localhost", false),
            ("host-1", true),
            ("0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43", true),
            ("Localhost", true),
        ];
        for (host, remote) in CASES {
            assert_eq!(is_remote_host(host), *remote, "{host:?}");
        }
    }
}
