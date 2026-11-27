#[derive(Clone, Copy, PartialEq, Eq)]
pub(crate) enum Ipv6Form {
    Yes,
    No,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_forms_differ() {
        assert!(Ipv6Form::Yes != Ipv6Form::No);
    }
}
