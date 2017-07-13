package main

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestParseGenericParams(t *testing.T) {
	cases := []struct {
		expected *genericParam
		desc     string
	}{
		{&genericParam{t: "Ljava/lang/Object;"}, "Ljava/lang/Object;"},

		{&genericParam{
			t: "Lscala/Option<>;",
			params: []genericParam{
				{t: "Ljava/lang/Object;"},
			},
		}, "Lscala/Option<Ljava/lang/Object;>;"},

		{&genericParam{
			t: "Lscala/Option<>;",
			params: []genericParam{
				{
					t: "Lscala/collection/immutable/List<>;",
					params: []genericParam{
						{t: "Ljava/lang/Object;"},
					},
				},
			},
		}, "Lscala/Option<Lscala/collection/immutable/List<Ljava/lang/Object;>;>;"},

		{&genericParam{
			t: "Lscala/collection/immutable/Map<>;",
			params: []genericParam{
				{t: "Ljava/lang/String;"},
				{t: "Ljava/lang/String;"},
			},
		}, "Lscala/collection/immutable/Map<Ljava/lang/String;Ljava/lang/String;>;"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d %s", i, c.desc), func(t *testing.T) {
			assert := assert.New(t)
			actual, err := parseGenericParams(c.desc)
			assert.NoError(err)
			assert.Equal(c.expected, actual)
		})
	}
}

func TestGoType(t *testing.T) {
	g := &generator{
		out:        ioutil.Discard,
		classNames: []string{"com.ua.Foo"},
	}

	cases := []struct {
		expected  string
		scalaType string
	}{
		{"map[string]string", "Lscala/collection/immutable/Map<Ljava/lang/String;Ljava/lang/String;>;"},
		{"[]int", "Lscala/Option<Lscala/collection/immutable/List<Ljava/lang/Object;>;>;"},
		//{"*bool", "Lscala/Option<Z>;"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d %s", i, c.scalaType), func(t *testing.T) {
			assert := assert.New(t)
			actual, err := g.goType(c.scalaType)
			assert.NoError(err)
			assert.Equal(c.expected, actual)
		})
	}
}
