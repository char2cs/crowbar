use super::*;

#[test]
fn a_capable_provider_converts_to_available() {
    assert_eq!(TerminalThreads::from(true), TerminalThreads::Available);
    assert_eq!(TerminalThreads::from(false), TerminalThreads::Unavailable);
}
