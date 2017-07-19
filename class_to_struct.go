package main

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/paultyng/jclass"
	"github.com/pkg/errors"
)

type blacklistError struct{}

func (e *blacklistError) Error() string {
	return "blacklist"
}

var builtinTypes = map[string]string{
	"Lscala/collection/immutable/List<>;": "[]%s",
	"Lscala/collection/immutable/Map<>;":  "map[%s]%s",
	"Lscala/collection/immutable/Set<>;":  "[]%s",
	"Lscala/collection/Seq<>;":            "[]%s",

	// simple cases
	// B	byte	signed byte
	// C	char	Unicode character code point in the Basic Multilingual Plane, encoded with UTF-16
	// D	double	double-precision floating-point value
	// F	float	single-precision floating-point value
	// L	ClassName ;	reference	an instance of class ClassName
	// S	short	signed short
	// [	reference	one array dimension
	"J": "int64",
	"I": "int",
	"Z": "bool",

	"Ljava/lang/Object;":        "int",
	"Ljava/lang/String;":        "string",
	"Ljava/sql/Date;":           "time.Time",
	"Ljava/sql/Timestamp;":      "time.Time",
	"Lorg/joda/time/DateTime;":  "time.Time",
	"Lorg/joda/time/LocalDate;": "time.Time",
	"Lscala/Enumeration$Value;": "string",
	"Lscala/math/BigDecimal;":   "decimal.Decimal",
}

type generator struct {
	classNames      []string
	out             io.Writer
	customTypes     map[string]string
	caseOverrides   []string
	blacklistTypes  []string
	blacklistFields []string
}

func (g *generator) Printf(f string, a ...interface{}) {
	fmt.Fprintf(g.out, f, a...)
}

type genericParam struct {
	t      string
	params []genericParam
}

func parseGenericParams(desc string) (*genericParam, error) {
	if i := strings.Index(desc, "<"); i >= 0 {
		p := &genericParam{
			t:      desc[0:i+1] + ">;",
			params: []genericParam{},
		}
		inner := desc[i+1 : len(desc)-2]
		opens := 0
		pos := strings.IndexAny(inner, ";<>")
		lastPos := 0
		for pos >= 0 {
			switch c := inner[pos]; c {
			case '<':
				opens++
			case '>':
				opens--
			case ';':
				if opens == 0 {
					child, err := parseGenericParams(inner[lastPos : pos+1])
					if err != nil {
						return nil, err
					}
					p.params = append(p.params, *child)
					lastPos = pos + 1
				}
			}
			if pos >= len(inner)-1 {
				break
			}

			pos += strings.IndexAny(inner[pos+1:len(inner)], ";<>") + 1
		}

		return p, nil
	}

	return &genericParam{
		t: desc,
	}, nil
}

func (g *generator) mapScalaType(gp genericParam) (string, error) {
	children := make([]interface{}, len(gp.params))
	for i, st := range gp.params {
		c, err := g.mapScalaType(st)
		if err != nil {
			return "", err
		}
		children[i] = c
	}

	desc := gp.t
	//HACK not sure why these are lower cased from viper
	if t, ok := g.customTypes[strings.ToLower(desc)]; ok {
		if strings.Contains(desc, "%") {
			return fmt.Sprintf(t, children...), nil
		}
		return t, nil
	}
	for _, bT := range g.blacklistTypes {
		if bT == desc {
			return "", &blacklistError{}
		}
		continue
	}
	//special scala.Option handling for maps/slices and pointers
	if desc == "Lscala/Option<>;" {
		if len(children) != 1 {
			return "", errors.Errorf("scala.Option requires 1 generic parameter")
		}
		c := children[0].(string)
		if strings.HasPrefix(c, "[]") || strings.HasPrefix(c, "map[") {
			return c, nil
		}
		return "*" + c, nil
	}
	if t, ok := builtinTypes[desc]; ok {
		return fmt.Sprintf(t, children...), nil
	}
	if strings.HasPrefix(desc, "L") {
		// is it a class we are processing?
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

func (g *generator) goType(desc string) (string, error) {
	gp, err := parseGenericParams(desc)
	if err != nil {
		return "", err
	}

	dst, err := g.mapScalaType(*gp)
	if err != nil {
		return "", err
	}
	return dst, nil
}

func (g *generator) fieldType(f *jclass.FieldInfo) (string, error) {
	var sig *jclass.ConstantUtf8Info
	for _, ai := range f.Attributes {
		if ai.NameString() == "Signature" {
			cpi := binary.BigEndian.Uint16(ai.Info)
			cp := ai.ConstantPoolInfo(cpi)
			sig = (*jclass.ConstantUtf8Info)(cp)
		}
	}

	desc := f.DescriptorString()
	if sig != nil {
		desc = sig.Utf8()
	}

	return g.goType(desc)
}

func splitOnBoundary(s string) []string {
	runes := []rune(s)

	if len(runes) <= 1 {
		return []string{s}
	}

	current := string(runes[0])
	parts := []string{}
	for i := 1; i < len(runes); i++ {
		switch {
		// Boundary is... aB
		case unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]):
			fallthrough
		// Boundary is #(a|A)
		case unicode.IsNumber(runes[i]) && unicode.IsLetter(runes[i-1]):
			fallthrough
		// Boundary is (a|A)#
		case unicode.IsLetter(runes[i]) && unicode.IsNumber(runes[i-1]):
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
	parts := splitOnBoundary(name)
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
	parts := splitOnBoundary(name)
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
	for _, bl := range g.blacklistFields {
		if strings.ToLower(bl) == strings.ToLower(name) {
			return "", "", "", &blacklistError{}
		}
	}
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
	var scala bool
	for _, ai := range cf.Attributes {
		if ai.NameString() == "ScalaSig" {
			scala = true
		}
		if ai.NameString() == "RuntimeVisibleAnnotations" {
			//TODO:
		}
	}
	if !scala {
		return errors.Errorf("class does not have a ScalaSig")
	}
	for _, field := range cf.Fields {
		if field.AccessFlags&privateFinal != 0 {
			n, t, extra, err := g.structField(field)
			if err != nil {
				switch err := err.(type) {
				case *blacklistError:
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
