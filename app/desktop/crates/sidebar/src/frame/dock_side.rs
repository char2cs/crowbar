use serde::{Deserialize, Serialize};

const LEFT_SIGN: f32 = 1.0;
const RIGHT_SIGN: f32 = -1.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub enum DockSide {
    #[default]
    Left,
    Right,
}

impl DockSide {
    pub(crate) fn growth_sign(self) -> f32 {
        match self {
            Self::Left => LEFT_SIGN,
            Self::Right => RIGHT_SIGN,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_edge_grows_toward_itself() {
        const CASES: [(DockSide, f32); 2] = [(DockSide::Left, 1.0), (DockSide::Right, -1.0)];
        for (side, sign) in CASES {
            assert!((side.growth_sign() - sign).abs() < f32::EPSILON, "{side:?}");
        }
    }
}
