package fakerjs_test

import (
	"errors"
	"testing"

	"github.com/iamramtin/go-fakerjs"
)

func TestErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		err  *fakerjs.Error
		want string
	}{
		{
			"a JavaScript exception names the method",
			&fakerjs.Error{Class: fakerjs.ClassJSException, Method: "internet.jwt", Msg: "ReferenceError: TextEncoder is not defined"},
			"failed to execute faker.internet.jwt: ReferenceError: TextEncoder is not defined",
		},
		{
			"a message wins over the sentinel",
			&fakerjs.Error{Class: fakerjs.ClassEngine, Msg: "engine start-up panicked: boom", Err: fakerjs.ErrPoisoned},
			"engine start-up panicked: boom",
		},
		{
			"without a message, the sentinel's text",
			&fakerjs.Error{Class: fakerjs.ClassResource, Err: fakerjs.ErrDeadline},
			"faker call exceeded its deadline",
		},
		{
			"with neither, the class",
			&fakerjs.Error{Class: fakerjs.ClassEngine},
			"engine",
		},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("%s: Error() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestErrorMatchesItsSentinelAndClass(t *testing.T) {
	err := error(&fakerjs.Error{Class: fakerjs.ClassResource, Err: fakerjs.ErrDeadline})

	if !errors.Is(err, fakerjs.ErrDeadline) {
		t.Error("errors.Is does not match the sentinel")
	}

	if !fakerjs.IsClass(err, fakerjs.ClassResource) || fakerjs.IsClass(err, fakerjs.ClassEngine) {
		t.Error("IsClass does not report the class exactly")
	}

	if fakerjs.IsClass(errors.New("plain"), fakerjs.ClassResource) {
		t.Error("IsClass matched an error that is not a *fakerjs.Error")
	}
}

func TestPoolSize(t *testing.T) {
	for _, c := range []struct{ asked, want int }{{3, 3}, {1, 1}, {0, 1}, {-2, 1}} {
		p := fakerjs.NewPool(c.asked, 42)
		if got := p.Size(); got != c.want {
			t.Errorf("NewPool(%d, 42).Size() = %d, want %d", c.asked, got, c.want)
		}

		p.Close()
	}
}
