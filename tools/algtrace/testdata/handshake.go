package protocol

func Publish(out chan<- string, state *int) {
	*state = 1
	out <- "ready"
}
