use std::fmt::Display;

use crate::Level;

pub trait Event: Display {
    fn source(&self) -> &'static str;

    fn name(&self) -> &'static str;

    fn level(&self) -> Level;
}
