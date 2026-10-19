package sample

func T(string) {}

func caller[T ~string](value string) {
	_ = T(value)
}
