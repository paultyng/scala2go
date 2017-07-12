package main

import (
	"fmt"
	"testing"

	"io/ioutil"

	"github.com/magiconair/properties/assert"
)

func TestStructName(t *testing.T) {
	g := &generator{
		out:        ioutil.Discard,
		classNames: []string{"com.ua.Foo"},
	}

	cases := []struct {
		expected  string
		className string
	}{
		{"Account", "com.ua.b2bservice.model.Account"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d %s", i, c.className), func(t *testing.T) {
			actual := g.structName(c.className)
			assert.Equal(t, c.expected, actual)
		})
	}
}
