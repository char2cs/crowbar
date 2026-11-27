#[derive(Debug, Clone, Copy, PartialEq, Eq, thiserror::Error)]
pub(crate) enum EndpointError {
    #[error(
        "Enter a host name or IP address, optionally with a port, like homelab.example.com:3737."
    )]
    NotAnAddress,
    #[error("The port must be a number from 1 to 65535.")]
    NotAPort,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_reason_reads_as_the_advice_shown_to_the_user() {
        assert!(
            EndpointError::NotAnAddress
                .to_string()
                .starts_with("Enter a host name")
        );
        assert!(EndpointError::NotAPort.to_string().contains("1 to 65535"));
    }
}
