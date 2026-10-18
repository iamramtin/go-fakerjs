package main

import (
	"fmt"

	"github.com/iamramtin/go-fakerjs"
)

func main() {
	e := fakerjs.NewEngine(1)
	v, _ := e.Generate("person.firstName", nil)
	fmt.Println(v)
}
