use crate::values::Key;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Action {
    ResetOverrides(Key),
    Backend(String),
}

impl Action {
    pub(crate) fn backend(id: impl Into<String>) -> Self {
        Self::Backend(id.into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backend_actions_carry_their_id() {
        assert_eq!(
            Action::backend("hosts.add"),
            Action::Backend("hosts.add".to_owned())
        );
    }
}
