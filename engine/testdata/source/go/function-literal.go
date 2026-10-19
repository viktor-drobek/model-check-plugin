package sample

func caller() {
	func() {
		unknown()
	}()
}
