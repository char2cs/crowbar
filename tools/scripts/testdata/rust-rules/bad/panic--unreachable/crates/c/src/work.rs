/// Works.
pub fn work(x: u32) -> u32 {
    match x {
        1 => 2,
        _ => unreachable!(),
    }
}

#[cfg(test)]
mod tests {}
