use super::*;

#[test]
fn linux_uses_control() {
    if cfg!(target_os = "linux") {
        assert_eq!(Primary::native(), Primary::Control);
    }
}
