//! The bounds, step and default of a numeric setting, named as data.

use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::NumberRange;
use crate::schema::OnLoad;

#[derive(Debug, Clone, Copy)]
pub(in crate::catalog) struct NumberSetting {
    range: (f64, f64, f64),
    pub(in crate::catalog) default: f64,
    on_load: OnLoad,
}

impl NumberSetting {
    pub(in crate::catalog) const fn new(
        range: (f64, f64, f64),
        default: f64,
    ) -> Self {
        Self {
            range,
            default,
            on_load: OnLoad::Kept,
        }
    }

    pub(in crate::catalog) const fn normalized(
        range: (f64, f64, f64),
        default: f64,
    ) -> Self {
        Self::new(range, default).loaded_as(OnLoad::Normalized)
    }

    pub(in crate::catalog) const fn loaded_as(
        mut self,
        on_load: OnLoad,
    ) -> Self {
        self.on_load = on_load;
        self
    }

    fn number_range(self) -> NumberRange {
        let (min, max, step) = self.range;
        NumberRange::new(min, max, step).loaded_as(self.on_load)
    }

    pub(in crate::catalog) fn plain(
        self,
        width: ControlWidth,
    ) -> Control {
        Control::number(self.number_range(), width)
    }

    pub(in crate::catalog) fn snapped(
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
