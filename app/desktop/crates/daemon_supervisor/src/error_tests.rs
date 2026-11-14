use std::io::ErrorKind;

use super::*;

fn os(errno: Errno) -> std::io::Error {
    std::io::Error::from_raw_os_error(errno.raw_os_error())
}

#[test]
fn running_out_of_descriptors_is_recognised_wherever_it_sits_in_the_chain() {
    let direct = Error::Dial {
        source: os(Errno::MFILE),
    };
    let system_wide = Error::Dial {
        source: os(Errno::NFILE),
    };
    let nested = Error::Exchange {
        target: "/v0/health".into(),
        source: Box::new(os(Errno::MFILE)),
    };
    for error in [direct, system_wide, nested] {
        assert!(error.is_descriptor_exhaustion(), "{error:?}");
    }
}

#[test]
fn other_failures_are_not_descriptor_exhaustion() {
    let cases = [
        Error::Dial {
            source: os(Errno::CONNREFUSED),
        },
        Error::Dial {
            source: std::io::Error::from(ErrorKind::NotFound),
        },
        Error::Status {
            target: "/x".into(),
            status: 503,
        },
        Error::GaveUp(3),
    ];
    for error in cases {
        assert!(!error.is_descriptor_exhaustion(), "{error:?}");
    }
}
