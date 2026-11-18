package clicore

import (
	_ "embed"
	"os"
	"reflect"
	"strings"
	"text/template"
)

//go:embed python_api_template.txt
var pythonAPITemplate string

type pythonMethod struct {
	PythonName  string // e.g. : "init", "handle_add", "handle_push"
	Description string
	Usage       string
	CmdParts    []string // e.g. : ["handle", "add"]
	Params      []pythonParam
	ResultType  *pythonResultType
}

type pythonParam struct {
	Name         string
	Type         string // Python type: "str", "bool"
	Default      string // Python default value
	Optional     bool
	IsPositional bool
	IsFlag       bool
	IsVariadic   bool   // Python *args (trailing list of positionals)
	FlagName     string // Flag name (without -)
	FlagKind     string // "bool" (flag alone), "str" (-name value), "list" (-name a,b)
	ParamDoc     string
}

type pythonResultType struct {
	TypeName string // e.g., "AddResult", "InitResult"
	Fields   []pythonResultField
}

type pythonResultField struct {
	Name       string // Field name (Python)
	JSONName   string // JSON tag name
	PythonType string // Python type annotation
}

type pythonAPIData struct {
	Methods           []pythonMethod
	UniqueResultTypes []*pythonResultType
}

// goTypeToPython maps go types to python types.
func goTypeToPython(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "str"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.Slice, reflect.Array:
		elemType := goTypeToPython(t.Elem())
		return "List[" + elemType + "]"
	case reflect.Map:
		keyType := goTypeToPython(t.Key())
		valType := goTypeToPython(t.Elem())
		return "Dict[" + keyType + ", " + valType + "]"
	default:
		return "Any"
	}
}

// extractResultType extracts Python type information from a Go ResultType using reflection
func extractResultType(resultType any) *pythonResultType {
	if resultType == nil {
		return nil
	}

	t := reflect.TypeOf(resultType)
	if t.Kind() != reflect.Struct {
		return nil
	}

	typeName := t.Name()
	fields := []pythonResultField{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		jsonName := strings.Split(jsonTag, ",")[0]
		pythonType := goTypeToPython(field.Type)
		fields = append(fields, pythonResultField{
			Name:       field.Name,
			JSONName:   jsonName,
			PythonType: pythonType,
		})
	}

	return &pythonResultType{
		TypeName: typeName,
		Fields:   fields,
	}
}

// GenPythonAPI generates a Python API file from the command tree
func (c *CmdTree) GenPythonAPI(outputPath string) error {
	data := pythonAPIData{
		Methods:           []pythonMethod{},
		UniqueResultTypes: []*pythonResultType{},
	}

	for _, cmd := range c.walkAPICommands("", nil) {
		cmd.Tree.collectSingleMethod(cmd.Name, cmd.CmdParts, &data.Methods)
	}

	seenTypes := make(map[string]bool)
	for _, method := range data.Methods {
		if method.ResultType != nil {
			typeName := method.ResultType.TypeName
			if !seenTypes[typeName] {
				seenTypes[typeName] = true
				data.UniqueResultTypes = append(data.UniqueResultTypes, method.ResultType)
			}
		}
	}

	tmpl, err := template.New("python_api").Parse(pythonAPITemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.Execute(f, data)
	if err != nil {
		return err
	}

	return os.Chmod(outputPath, 0o755)
}

func (c *CmdTree) collectSingleMethod(pythonName string, cmdParts []string, methods *[]pythonMethod) {
	method := pythonMethod{
		PythonName:  pythonName,
		Description: c.Description,
		CmdParts:    cmdParts,
		Params:      []pythonParam{},
		ResultType:  extractResultType(c.ResultType),
	}

	// Add positional parameters (required)
	for _, posArg := range c.ArgParser.PosArgs {
		method.Params = append(method.Params, pythonParam{
			Name:         posArg,
			Type:         "str",
			Optional:     false,
			IsPositional: true,
			IsFlag:       false,
			ParamDoc:     "Positional argument",
		})
	}

	// Add optional positional parameters
	for _, optArg := range c.ArgParser.OptionalPosArgs {
		method.Params = append(method.Params, pythonParam{
			Name:         optArg,
			Type:         "Optional[str]",
			Default:      "None",
			Optional:     true,
			IsPositional: true,
			IsFlag:       false,
			ParamDoc:     "Optional positional argument",
		})
	}

	// Add the variadic parameter (Python *args). Must come after the required
	// positionals and before flags (which become keyword-only after *args).
	// A variadic forbids optionals, so there are none between them.
	if c.ArgParser.variadicDefined {
		method.Params = append(method.Params, pythonParam{
			Name:         c.ArgParser.variadicDef.name,
			Type:         "str",
			Optional:     false,
			IsPositional: false,
			IsFlag:       false,
			IsVariadic:   true,
			ParamDoc:     c.ArgParser.variadicDef.usage,
		})
	}

	// Add flag parameters
	for _, flagName := range c.ArgParser.apiFlagNames() {
		flagDef := c.ArgParser.flagDefs[flagName]
		pythonFlagName := strings.ReplaceAll(flagName, "-", "_")

		switch flagDef.flagType {
		// TODO(cli): implement other types... int !
		case "bool":
			method.Params = append(method.Params, pythonParam{
				Name:     pythonFlagName,
				Type:     "bool",
				Default:  "False",
				Optional: true,
				IsFlag:   true,
				FlagName: flagName,
				FlagKind: "bool",
				ParamDoc: flagDef.usage,
			})
		case "string":
			method.Params = append(method.Params, pythonParam{
				Name:     pythonFlagName,
				Type:     "Optional[str]",
				Default:  "None",
				Optional: true,
				IsFlag:   true,
				FlagName: flagName,
				FlagKind: "str",
				ParamDoc: flagDef.usage,
			})
		case "commastring":
			method.Params = append(method.Params, pythonParam{
				Name:     pythonFlagName,
				Type:     "Optional[List[str]]",
				Default:  "None",
				Optional: true,
				IsFlag:   true,
				FlagName: flagName,
				FlagKind: "list",
				ParamDoc: flagDef.usage,
			})
		}
	}

	*methods = append(*methods, method)
}
