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

func TestSplitOnBoundary(t *testing.T) {
	cases := []struct {
		split     []string
		fieldName string
	}{
		{[]string{"Field", "One", "Two", "Three"}, "FieldOneTwoThree"},
		{[]string{"Field", "One", "Two"}, "FieldOneTwo"},
		{[]string{"Field", "One"}, "FieldOne"},
		{[]string{"Field", "1"}, "Field1"},
		{[]string{"Field", "123"}, "Field123"},
		{[]string{"Field", "123", "Four"}, "Field123Four"},
		{[]string{"B"}, "B"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d, %s", i, c.fieldName), func(t *testing.T) {
			assert := assert.New(t)
			actual := splitOnBoundary(c.fieldName)
			assert.Equal(len(actual), len(c.split))
		})
	}
}

func TestJSONName(t *testing.T) {
	g := &generator{
		out:        ioutil.Discard,
		classNames: []string{"com.ua.Foo"},
	}
	cases := []struct {
		jsonName  string
		fieldName string
	}{
		{"field_one_two_three", "FieldOneTwoThree"},
		{"field_one_two", "FieldOneTwo"},
		{"field_one", "FieldOne"},
		{"field_1", "Field1"},
		{"field_123", "Field123"},
		{"field_123_four", "Field123Four"},
		{"a", "A"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d, %s", i, c.fieldName), func(t *testing.T) {
			assert := assert.New(t)
			actual := g.jsonName(c.fieldName)
			assert.Equal(actual, c.jsonName)
		})
	}
}
