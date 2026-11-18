//! `Debug` for types that hold closures or elements and so cannot derive it.

/// Implements `Debug` printing only the type name.
macro_rules! opaque_debug {
    ($($ty:ident),+ $(,)?) => {$(
        impl ::std::fmt::Debug for $ty {
            fn fmt(&self, formatter: &mut ::std::fmt::Formatter<'_>) -> ::std::fmt::Result {
                formatter.debug_struct(stringify!($ty)).finish_non_exhaustive()
            }
        }
    )+};
}

pub(crate) use opaque_debug;

#[cfg(test)]
mod tests {
    struct Holder {
        _callback: Box<dyn Fn()>,
    }

    opaque_debug!(Holder);

    #[test]
    fn prints_only_the_type_name() {
        let holder = Holder {
            _callback: Box::new(|| {}),
        };
        assert_eq!(format!("{holder:?}"), "Holder { .. }");
    }
}
