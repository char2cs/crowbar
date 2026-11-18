//! The animated values of a toast: shake, pulse and fade.

use crate::display::toast::toast_kind::ToastKind;
use crate::display::toast::toast_phase::ToastPhase;
use crate::foundation::Keyframes;

pub(crate) fn shake_offset(
    kind: ToastKind,
    updates: u32,
    delta: f32,
) -> f32 {
    if kind != ToastKind::Error || updates == 0 {
        return 0.0;
    }
    Keyframes::TOAST_ERROR.sample(delta)
}

pub(crate) fn pulse_scale(
    kind: ToastKind,
    updates: u32,
    delta: f32,
) -> f32 {
    if kind != ToastKind::Success || updates == 0 {
        return 1.0;
    }
    Keyframes::TOAST_SUCCESS.sample(delta)
}

pub(crate) fn fade(
    phase: ToastPhase,
    delta: f32,
) -> f32 {
    match phase {
        ToastPhase::Shown => delta,
        ToastPhase::Leaving => 1.0 - delta,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_updated_errors_shake_and_only_updated_successes_pulse() {
        type Case = (ToastKind, u32, f32, (f32, f32));
        const CASES: &[Case] = &[
            (ToastKind::Error, 0, 0.25, (0.0, 1.0)),
            (ToastKind::Error, 1, 0.25, (-3.0, 1.0)),
            (ToastKind::Success, 1, 0.3, (0.0, 1.025)),
            (ToastKind::Success, 1, 0.6, (0.0, 0.99)),
            (ToastKind::Success, 0, 0.3, (0.0, 1.0)),
            (ToastKind::Info, 1, 0.3, (0.0, 1.0)),
        ];
        for &(kind, updates, delta, (shake, pulse)) in CASES {
            let moved = shake_offset(kind, updates, delta);
            assert!(
                (moved - shake).abs() < 1e-3,
                "{kind:?} {updates} shake {moved}"
            );
            let scaled = pulse_scale(kind, updates, delta);
            assert!(
                (scaled - pulse).abs() < 1e-3,
                "{kind:?} {updates} pulse {scaled}"
            );
        }
    }

    #[test]
    fn fade_runs_in_for_a_new_toast_and_out_for_a_leaving_one() {
        const CASES: &[(ToastPhase, f32, f32)] = &[
            (ToastPhase::Shown, 0.0, 0.0),
            (ToastPhase::Shown, 1.0, 1.0),
            (ToastPhase::Leaving, 0.0, 1.0),
            (ToastPhase::Leaving, 1.0, 0.0),
        ];
        for &(phase, delta, opacity) in CASES {
            assert!(
                (fade(phase, delta) - opacity).abs() < 1e-6,
                "{phase:?} {delta}"
            );
        }
    }
}
