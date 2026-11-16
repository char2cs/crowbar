/// Works.
pub fn work(a: u32) -> u32 {
    let pick = |flag: bool| flag;
    u32::from(pick(a > 1))
}

#[cfg(test)]
mod tests {}
