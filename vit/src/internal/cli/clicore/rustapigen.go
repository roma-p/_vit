package clicore

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"
	"unicode"

	"vit/internal/fsutil"
	"vit/internal/types"
)

//go:embed rust_api_template.txt
var rustAPITemplate string

type rustAPIData struct {
	Methods []rustMethod
	Structs []*rustStruct
}

type rustMethod struct {
	Name        string
	Description string
	CmdParts    []string
	Positionals []rustParam
	Optionals   []rustParam
	Variadic    *rustParam
	OptsType    string // "" if the command has no flag.
	Flags       []rustFlag
	ResultType  string
}

type rustParam struct {
	Name string
}

type rustFlag struct {
	Name     string
	FlagName string
	Kind     string // "bool", "str", "list"
	Doc      string
}

type rustStruct struct {
	Name   string
	Fields []rustField
}

type rustField struct {
	Name     string
	JSONName string
	Type     string
}

// GenRustAPI generates the Rust bindings (one lib.rs) from the command tree.
func (c *CmdTree) GenRustAPI(outputPath string) error {
	reg := newRustTypeRegistry()
	data := rustAPIData{}
	for _, cmd := range c.walkAPICommands("", nil) {
		method, err := cmd.rustMethod(reg)
		if err != nil {
			return fmt.Errorf("command '%s': %w", cmd.Name, err)
		}
		data.Methods = append(data.Methods, method)
	}
	// Progress events are part of every command's output.
	for _, t := range []any{types.ProgressManifest{}, types.ProgressItem{}, types.ProgressFinish{}} {
		if _, err := reg.typeOf(reflect.TypeOf(t)); err != nil {
			return err
		}
	}
	data.Structs = reg.structs

	tmpl, err := template.New("rust_api").Delims("<%", "%>").Parse(rustAPITemplate)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(fsutil.DirOS(outputPath), 0o755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(outputPath, 0o644, func(f *os.File) error {
		return tmpl.Execute(f, data)
	})
}

func (cmd apiCommand) rustMethod(reg *rustTypeRegistry) (rustMethod, error) {
	parser := cmd.Tree.ArgParser
	method := rustMethod{
		Name:        rustIdent(cmd.Name),
		Description: cmd.Tree.Description,
		CmdParts:    cmd.CmdParts,
		ResultType:  "serde_json::Value",
	}
	for _, arg := range parser.PosArgs {
		method.Positionals = append(method.Positionals, rustParam{Name: rustIdent(arg)})
	}
	for _, arg := range parser.OptionalPosArgs {
		method.Optionals = append(method.Optionals, rustParam{Name: rustIdent(arg)})
	}
	if parser.variadicDefined {
		method.Variadic = &rustParam{Name: rustIdent(parser.variadicDef.name)}
	}
	for _, flagName := range parser.apiFlagNames() {
		def := parser.flagDefs[flagName]
		flag := rustFlag{Name: rustIdent(flagName), FlagName: flagName, Doc: def.usage}
		switch def.flagType {
		case "bool":
			if def.defaultVal == true {
				return method, fmt.Errorf("flag '%s': bool defaulting to true is not supported", flagName)
			}
			flag.Kind = "bool"
		case "string":
			flag.Kind = "str"
		case "commastring":
			flag.Kind = "list"
		default:
			return method, fmt.Errorf("flag '%s': unsupported type '%s'", flagName, def.flagType)
		}
		method.Flags = append(method.Flags, flag)
	}
	if len(method.Flags) > 0 {
		method.OptsType = rustCamel(cmd.Name) + "Opts"
	}
	if cmd.Tree.ResultType != nil {
		resultType, err := reg.typeOf(reflect.TypeOf(cmd.Tree.ResultType))
		if err != nil {
			return method, err
		}
		method.ResultType = resultType
	}
	return method, nil
}

// rustTypeRegistry walks Go types by reflection and collects the structs they
// reach, following what encoding/json does with each of them.
type rustTypeRegistry struct {
	structs []*rustStruct
	names   map[string]reflect.Type
	active  map[reflect.Type]bool // structs being walked: a pointer to one is a cycle.
}

func newRustTypeRegistry() *rustTypeRegistry {
	return &rustTypeRegistry{
		names:  make(map[string]reflect.Type),
		active: make(map[reflect.Type]bool),
	}
}

func (r *rustTypeRegistry) typeOf(t reflect.Type) (string, error) {
	if t == reflect.TypeOf(time.Time{}) {
		return "String", nil
	}
	switch t.Kind() {
	case reflect.String:
		return "String", nil
	case reflect.Bool:
		return "bool", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "i64", nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "u64", nil
	case reflect.Float32, reflect.Float64:
		return "f64", nil
	case reflect.Interface:
		return "serde_json::Value", nil
	case reflect.Pointer:
		inner, err := r.typeOf(t.Elem())
		if err != nil {
			return "", err
		}
		if r.active[t.Elem()] {
			return "Option<Box<" + inner + ">>", nil
		}
		return "Option<" + inner + ">", nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "String", nil // base64 in JSON
		}
		inner, err := r.typeOf(t.Elem())
		if err != nil {
			return "", err
		}
		return "Vec<" + inner + ">", nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return "", fmt.Errorf("map key %s: only string keys are supported", t.Key())
		}
		inner, err := r.typeOf(t.Elem())
		if err != nil {
			return "", err
		}
		return "std::collections::BTreeMap<String, " + inner + ">", nil
	case reflect.Struct:
		return r.structOf(t)
	default:
		return "", fmt.Errorf("type %s: unsupported kind %s", t, t.Kind())
	}
}

func (r *rustTypeRegistry) structOf(t reflect.Type) (string, error) {
	name := t.Name()
	if name == "" {
		return "", fmt.Errorf("anonymous struct %s is not supported", t)
	}
	if seen, ok := r.names[name]; ok {
		if seen != t {
			return "", fmt.Errorf("two types named %s: %s and %s", name, seen, t)
		}
		return name, nil
	}
	r.names[name] = t
	r.active[t] = true
	defer delete(r.active, t)

	s := &rustStruct{Name: name}
	r.structs = append(r.structs, s)
	fields, err := r.fieldsOf(t)
	if err != nil {
		return "", err
	}
	s.Fields = fields
	return name, nil
}

func (r *rustTypeRegistry) fieldsOf(t reflect.Type) ([]rustField, error) {
	var ret []rustField
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		jsonName, opts, _ := strings.Cut(tag, ",")
		if tag == "-" || !field.IsExported() && !field.Anonymous {
			continue
		}
		if strings.Contains(opts, "string") {
			return nil, fmt.Errorf("field %s.%s: json option 'string' is not supported", t, field.Name)
		}
		if field.Anonymous && jsonName == "" {
			if field.Type.Kind() != reflect.Struct {
				return nil, fmt.Errorf("embedded field %s.%s: only structs are supported", t, field.Name)
			}
			embedded, err := r.fieldsOf(field.Type)
			if err != nil {
				return nil, err
			}
			ret = append(ret, embedded...)
			continue
		}
		if jsonName == "" {
			jsonName = field.Name // what encoding/json does without a tag.
		}
		fieldType, err := r.typeOf(field.Type)
		if err != nil {
			return nil, fmt.Errorf("field %s.%s: %w", t, field.Name, err)
		}
		ret = append(ret, rustField{Name: rustIdent(jsonName), JSONName: jsonName, Type: fieldType})
	}
	return ret, nil
}

var rustKeywords = map[string]bool{
	"abstract": true, "as": true, "async": true, "await": true, "become": true, "box": true,
	"break": true, "const": true, "continue": true, "do": true, "dyn": true, "else": true,
	"enum": true, "extern": true, "false": true, "final": true, "fn": true, "for": true,
	"gen": true, "if": true, "impl": true, "in": true, "let": true, "loop": true, "macro": true,
	"match": true, "mod": true, "move": true, "mut": true, "override": true, "priv": true,
	"pub": true, "ref": true, "return": true, "static": true, "struct": true, "trait": true,
	"true": true, "try": true, "type": true, "typeof": true, "unsafe": true, "unsized": true,
	"use": true, "virtual": true, "where": true, "while": true, "yield": true,
}

// rustIdent returns the snake_case form of a name, valid as a Rust identifier.
func rustIdent(name string) string {
	snake := rustSnake(name)
	switch {
	case snake == "self" || snake == "super" || snake == "crate":
		return snake + "_"
	case rustKeywords[snake]:
		return "r#" + snake
	}
	return snake
}

// rustSnake converts branchPath, RepoUID and repo-uid to branch_path, repo_uid.
func rustSnake(name string) string {
	runes := []rune(strings.ReplaceAll(name, "-", "_"))
	var b strings.Builder
	for i, c := range runes {
		if unicode.IsUpper(c) && i > 0 && runes[i-1] != '_' {
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if !unicode.IsUpper(runes[i-1]) || nextLower {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
}

func rustCamel(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(rustSnake(name), "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}
