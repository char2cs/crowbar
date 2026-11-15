#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum EnvChange {
    Remove(&'static str),
    Set(&'static str, &'static str),
}

impl EnvChange {
    #[expect(
        unsafe_code,
        reason = "editing the environment is unsafe in edition 2024; callers run before any thread starts"
    )]
    pub(crate) fn apply(self) {
        match self {
            // SAFETY: the shell applies this in `main` before it starts any other thread.
            Self::Remove(key) => unsafe { std::env::remove_var(key) },
            // SAFETY: the shell applies this in `main` before it starts any other thread.
            Self::Set(key, value) => unsafe { std::env::set_var(key, value) },
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const PROBE: &str = "CROWBAR_ENV_CHANGE_PROBE";

    #[test]
    fn a_change_sets_and_removes_a_variable() {
        EnvChange::Set(PROBE, "on").apply();
        assert_eq!(std::env::var(PROBE).as_deref(), Ok("on"), "set");
        EnvChange::Remove(PROBE).apply();
        assert!(std::env::var_os(PROBE).is_none(), "removed");
    }
}
