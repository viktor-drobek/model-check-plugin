def consume(queue, state):
    while True:
        value = queue.get()
        if value == "stop":
            return
        state["last"] = value
