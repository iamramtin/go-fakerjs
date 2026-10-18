package main

import (
	"fmt"
	"time"

	"github.com/iamramtin/go-fakerjs/bench/qjsbase"
)

func main() {
	e, _ := qjsbase.New(1, time.Time{})
	v, _ := e.Generate("person.firstName", nil)
	fmt.Println(v)
}
