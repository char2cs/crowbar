//! The breakpoints the shell draws by, named as the pane wire names them.

pub(crate) fn wire_breakpoint(point: ui::Breakpoint) -> protocol::Breakpoint {
    match point {
        ui::Breakpoint::Sm => protocol::Breakpoint::Sm,
        ui::Breakpoint::Md => protocol::Breakpoint::Md,
        ui::Breakpoint::Lg => protocol::Breakpoint::Lg,
        ui::Breakpoint::Xl => protocol::Breakpoint::Xl,
    }
}

#[cfg(test)]
mod tests {
    use super::wire_breakpoint;

    fn drawn(point: protocol::Breakpoint) -> ui::Breakpoint {
        match point {
            protocol::Breakpoint::Sm => ui::Breakpoint::Sm,
            protocol::Breakpoint::Md => ui::Breakpoint::Md,
            protocol::Breakpoint::Lg => ui::Breakpoint::Lg,
            protocol::Breakpoint::Xl => ui::Breakpoint::Xl,
        }
    }

    #[test]
    fn the_drawn_and_wire_breakpoints_list_the_same_variants() {
        let wired: Vec<_> = ui::Breakpoint::ALL.map(wire_breakpoint).to_vec();
        let back: Vec<_> = wired.iter().copied().map(drawn).collect();
        assert_eq!(back, ui::Breakpoint::ALL);
        let names = serde_json::to_value(&wired).expect("the names serialize");
        assert_eq!(names, serde_json::json!(["sm", "md", "lg", "xl"]));
    }
}
