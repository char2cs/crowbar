#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DomOp {
    Snapshot,
    Click,
    Type,
    PressKey,
    Scroll,
    Wait,
    Resolve,
}

impl DomOp {
    pub(crate) fn of(op: &str) -> Option<Self> {
        match op {
            "snapshot" => Some(Self::Snapshot),
            "click" => Some(Self::Click),
            "type" => Some(Self::Type),
            "press_key" => Some(Self::PressKey),
            "scroll" => Some(Self::Scroll),
            "wait" => Some(Self::Wait),
            "resolve" => Some(Self::Resolve),
            _ => None,
        }
    }

    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::Snapshot => "snapshot",
            Self::Click => "click",
            Self::Type => "type",
            Self::PressKey => "press_key",
            Self::Scroll => "scroll",
            Self::Wait => "wait",
            Self::Resolve => "resolve",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_operation_is_known_by_the_name_the_daemon_sends() {
        for name in [
            "snapshot",
            "click",
            "type",
            "press_key",
            "scroll",
            "wait",
            "resolve",
        ] {
            assert_eq!(DomOp::of(name).map(DomOp::name), Some(name));
        }
        assert_eq!(DomOp::of("eval"), None);
    }
}
