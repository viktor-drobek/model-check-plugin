use std::sync::mpsc::Sender;

fn publish(tx: Sender<String>) {
    tx.send(String::from("ready")).unwrap();
}
