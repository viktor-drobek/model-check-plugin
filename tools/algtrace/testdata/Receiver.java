import java.util.concurrent.BlockingQueue;

class Receiver {
    String receive(BlockingQueue<String> queue) throws InterruptedException {
        return queue.take();
    }
}
