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
#[path = "opaque_debug_tests.rs"]
mod tests;
