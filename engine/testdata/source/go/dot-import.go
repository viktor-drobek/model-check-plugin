package sample

import . "fmt"

func Println(...any) {}

func caller() {
	Println("hello")
}
