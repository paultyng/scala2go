package main

import (
	"archive/zip"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/paultyng/jclass"
	"github.com/pkg/errors"
)

type blacklistedError struct{}

func (e *blacklistedError) Error() string {
	return "BLACKLISTED_FIELD"
}

type generator struct {
	classNames       []string
	out              io.Writer
	customTypes      map[string]string
	caseOverrides    []string
	blacklistedTypes []string
}

func (g *generator) Printf(f string, a ...interface{}) {
	fmt.Fprintf(g.out, f, a...)
}

func (g *generator) goType(desc string) (string, error) {
	//HACK not sure why these are lower cased from viper
	if t, ok := g.customTypes[strings.ToLower(desc)]; ok {
		return t, nil
	}
	for _, bT := range g.blacklistedTypes {
		if bT == desc {
			return "", &blacklistedError{}
		}
		continue
	}
	if i := strings.Index(desc, "<"); i >= 0 {
		wrapper := desc[0:i+1] + ">;"
		pDesc := desc[i+1 : len(desc)-2]

		p, err := g.goType(pDesc)
		if err != nil {
			return "", errors.Wrapf(err, "unable to find generic parameter %s", desc)
		}
		switch wrapper {
		case "Lscala/Option<>;":
			if strings.HasPrefix(p, "[]") || strings.HasPrefix(p, "map[") {
				return p, nil
			}
			return "*" + p, nil
		case "Lscala/collection/immutable/List<>;":
			return "[]" + p, nil
		case "Lscala/collection/Seq<>;":
			return "[]" + p, nil
		default:
			return "", errors.Errorf("unable to map type %s", desc)
		}
	}

	// simple cases
	switch desc {
	// B	byte	signed byte
	// C	char	Unicode character code point in the Basic Multilingual Plane, encoded with UTF-16
	// D	double	double-precision floating-point value
	// F	float	single-precision floating-point value
	// L	ClassName ;	reference	an instance of class ClassName
	// S	short	signed short
	// [	reference	one array dimension
	case "J":
		return "int64", nil
	case "I":
		return "int", nil
	case "Z":
		return "bool", nil
	case "Ljava/lang/Object;":
		return "int", nil
	case "Ljava/lang/String;":
		return "string", nil
	case "Lscala/Enumeration$Value;":
		return "string", nil
	case "Lscala/math/BigDecimal;":
		return "decimal.Decimal", nil
	case "Lorg/joda/time/DateTime;":
		return "time.Time", nil
	case "Ljava/sql/Timestamp;":
		return "time.Time", nil
	case "Ljava/sql/Date;":
		return "time.Time", nil
	}

	{
		test := desc
		test = strings.TrimPrefix(test, "L")
		test = strings.TrimSuffix(test, ";")
		test = strings.Replace(test, "/", ".", -1)
		for _, cn := range g.classNames {
			if cn == test {
				return g.structName(cn), nil
			}
		}
	}

	return "", errors.Errorf("unable to map type %s", desc)
}

func (g *generator) fieldType(f *jclass.FieldInfo) (string, error) {
	var sig *jclass.AttributeInfo
	for _, ai := range f.Attributes {
		if ai.NameString() == "Signature" {
			sig = ai
			break
		}
	}

	desc := f.DescriptorString()
	if sig != nil {
		desc = sig.SignatureString()
	}

	return g.goType(desc)
}

func splitOnCase(s string) []string {
	runes := []rune(s)

	if len(runes) <= 1 {
		return []string{s}
	}

	current := string(runes[0])
	parts := []string{}
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
			parts = append(parts, current)
			current = ""
		}
		current += string(runes[i])
	}
	if len(current) > 0 {
		parts = append(parts, current)
	}
	return parts
}

func (g *generator) fieldName(name string) string {
	parts := splitOnCase(name)
	newParts := make([]string, len(parts))

	for i, p := range parts {
		if commonInitialisms[strings.ToUpper(p)] {
			newParts[i] = strings.ToUpper(p)
			continue
		}

		r := []rune(p)
		newParts[i] = strings.ToUpper(string(r[0])) + string(r[1:])

		for _, co := range g.caseOverrides {
			if strings.ToLower(p) == strings.ToLower(co) {
				newParts[i] = co
				break
			}
		}
	}

	return strings.Join(newParts, "")
}

func (g *generator) jsonName(name string) string {
	parts := splitOnCase(name)
	newParts := make([]string, len(parts)*2-1)

	for i, p := range parts {
		newParts[i*2] = strings.ToLower(p)
		if i > 0 {
			newParts[i*2-1] = "_"
		}
	}

	return strings.Join(newParts, "")
}

func (g *generator) structField(f *jclass.FieldInfo) (name, goType, extra string, err error) {
	name = g.fieldName(f.NameString())
	t, err := g.fieldType(f)
	if err != nil {
		return "", "", "", err
	}
	jsonTags := []string{g.jsonName(f.NameString())}
	if strings.HasPrefix(t, "*") {
		jsonTags = append(jsonTags, "omitempty")
	}
	return name, t, fmt.Sprintf("`json:\"%s\"`", strings.Join(jsonTags, ",")), nil
}

func (g *generator) structName(name string) string {
	parts := strings.Split(name, ".")
	return parts[len(parts)-1]
}

func (g *generator) classFileToStruct(name string, f *zip.File) error {
	sn := g.structName(name)

	g.Printf("// %s represents the Scala class %s\n", sn, name)
	g.Printf("type %s struct {\n", sn)

	fr, err := f.Open()
	if err != nil {
		return errors.Wrapf(err, "unable to open class file %s", f.Name)
	}
	defer fr.Close()

	cf, err := jclass.NewClassFile(fr)
	if err != nil {
		return errors.Wrap(err, "unable to parse class file")
	}

	return g.classToStruct(name, cf)
}

func (g *generator) classToStruct(name string, cf *jclass.ClassFile) error {
	privateFinal := jclass.FIELD_ACC_PRIVATE + jclass.FIELD_ACC_FINAL
	sort.SliceStable(cf.Fields, func(i, j int) bool {
		return cf.Fields[i].NameString() < cf.Fields[j].NameString()
	})
	for _, field := range cf.Fields {
		if field.AccessFlags&privateFinal != 0 {
			n, t, extra, err := g.structField(field)
			if err != nil {
				switch err := err.(type) {
				case *blacklistedError:
					continue
				default:
					return errors.Wrapf(err, "unable to handle field %s", field.NameString())
				}
			}
			g.Printf("\t%s %s %s\n", n, t, extra)
		}
	}

	g.Printf("}\n\n")

	return nil
}
