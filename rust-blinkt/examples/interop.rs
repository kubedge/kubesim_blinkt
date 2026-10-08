//! Publishes or withdraws one owner's pixel through `ledstate` with a
//! recording renderer that prints each drawn frame as JSON. Used by
//! hack/interop to check the Rust and Go implementations share state correctly.
//!
//! interop <dir> <owner> publish <index> <r> <g> <b> [count]
//! interop <dir> <owner> withdraw

use std::collections::BTreeMap;
use std::time::Duration;

use kubesim_blinkt::ledstate::{Board, Frame, Pixel};

fn main() {
    let a: Vec<String> = std::env::args().collect();
    let num = |i: usize| a[i].parse::<i64>().expect("number");
    let print = |f: &Frame| -> Result<(), kubesim_blinkt::ledstate::Error> {
        println!("{}", serde_json::to_string(f)?);
        Ok(())
    };
    let mut board = Board::open(&a[1], &a[2], Duration::from_secs(60), print);
    assert!(board.solo_reason().is_none(), "{:?}", board.solo_reason());
    match a[3].as_str() {
        "withdraw" => board.withdraw().expect("withdraw"),
        _ => {
            let px = BTreeMap::from([(
                num(4) as usize,
                Pixel {
                    r: num(5),
                    g: num(6),
                    b: num(7),
                    l: 5,
                },
            )]);
            let count = a.get(8).map_or(1, |c| c.parse().expect("count"));
            for _ in 0..count {
                board.publish(&px).expect("publish");
            }
        }
    }
}
