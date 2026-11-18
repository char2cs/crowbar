//! The bounds, step and default of a numeric setting, named as data.

use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::NumberRange;

#[derive(Debug, Clone, Copy)]
pub(super) struct NumberSetting {
    range: (f64, f64, f64),
    pub(super) default: f64,
}

impl NumberSetting {
    pub(super) const fn new(
        range: (f64, f64, f64),
        default: f64,
    ) -> Self {
        Self { range, default }
    }

    fn number_range(self) -> NumberRange {
        let (min, max, step) = self.range;
        NumberRange::new(min, max, step)
    }

    pub(super) fn plain(
        self,
        width: ControlWidth,
    ) -> Control {
        Control::number(self.number_range(), width)
    }

    pub(super) fn snapped(
        self,
        width: ControlWidth,
    ) -> Control {
        Control::number(self.number_range().snapping(), width)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::values::Value;

    const SETTING: NumberSetting = NumberSetting::new((10.0, 24.0, 0.5), 15.0);

    #[test]
    fn snapped_numbers_snap_and_plain_numbers_do_not() {
        let snapping = SETTING.snapped(ControlWidth::Compact);
        let plain = SETTING.plain(ControlWidth::Compact);
        assert_eq!(snapping.parse("15.3"), Ok(Value::Number(15.5)));
        assert_eq!(plain.parse("15.3"), Ok(Value::Number(15.3)));
        assert!((SETTING.default - 15.0).abs() < f64::EPSILON);
    }
}
