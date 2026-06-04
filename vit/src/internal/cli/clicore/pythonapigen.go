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

type PythonMethod struct {
	PythonName  string // e.g. : "init", "handle_add", "handle_push"
	Description string
	Usage       string
	CmdParts    []string // e.g. : ["handle", "add"]
	Params      []PythonParam
	ResultType  *PythonResultType
}

type PythonParam struct {
	Name         string
	Type         string // Python type: "str", "bool"
	Default      string // Python default value
	Optional     bool
	IsPositional bool
	IsFlag       bool
	FlagName     string // Flag name (without -)
	ParamDoc     string
}

type PythonResultType struct {
	TypeName string // e.g., "AddResult", "InitResult"
	Fields   []PythonResultField
}

type PythonResultField struct {
	Name       string // Field name (Python)
	JSONName   string // JSON tag name
	PythonType string // Python type annotation
}

type PythonAPIData struct {
	Methods           []PythonMethod
	UniqueResultTypes []*PythonResultType
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
func extractResultType(resultType any) *PythonResultType {
	if resultType == nil {
		return nil
	}

	t := reflect.TypeOf(resultType)
	if t.Kind() != reflect.Struct {
		return nil
	}

	typeName := t.Name()
	fields := []PythonResultField{}

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
		fields = append(fields, PythonResultField{
			Name:       field.Name,
			JSONName:   jsonName,
			PythonType: pythonType,
		})
	}

	return &PythonResultType{
		TypeName: typeName,
		Fields:   fields,
	}
}

// GenPythonAPI generates a Python API file from the command tree
func (c *CmdTree) GenPythonAPI(outputPath string) error {
	data := PythonAPIData{
		Methods:           []PythonMethod{},
		UniqueResultTypes: []*PythonResultType{},
	}

	c.collectMethods("", []string{}, &data.Methods)

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

func (c *CmdTree) collectSingleMethod(pythonName string, cmdParts []string, methods *[]PythonMethod) {
	method := PythonMethod{
		PythonName:  pythonName,
		Description: c.Description,
		CmdParts:    cmdParts,
		Params:      []PythonParam{},
		ResultType:  extractResultType(c.ResultType),
	}

	// Add positional parameters (required)
	for _, posArg := range c.ArgParser.PosArgs {
		method.Params = append(method.Params, PythonParam{
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
		method.Params = append(method.Params, PythonParam{
			Name:         optArg,
			Type:         "Optional[str]",
			Default:      "None",
			Optional:     true,
			IsPositional: true,
			IsFlag:       false,
			ParamDoc:     "Optional positional argument",
		})
	}

	// Add flag parameters (skip global flags: json, debug, v, h)
	for flagName, flagDef := range c.ArgParser.flagDefs {
		// Skip global flags
		if flagName == "json" || flagName == "debug" || flagName == "v" || flagName == "h" {
			continue
		}

		pythonFlagName := strings.ReplaceAll(flagName, "-", "_")

		switch flagDef.flagType {
		// TODO(cli): implement other types... int !
		case "bool":
			method.Params = append(method.Params, PythonParam{
				Name:     pythonFlagName,
				Type:     "bool",
				Default:  "False",
				Optional: true,
				IsFlag:   true,
				FlagName: flagName,
				ParamDoc: flagDef.usage,
			})
		case "string":
			method.Params = append(method.Params, PythonParam{
				Name:     pythonFlagName,
				Type:     "Optional[str]",
				Default:  "None",
				Optional: true,
				IsFlag:   true,
				FlagName: flagName,
				ParamDoc: flagDef.usage,
			})
		}
	}

	*methods = append(*methods, method)
}

// collectMethods recursively walks the command tree and collects Python methods
func (c *CmdTree) collectMethods(pythonName string, cmdParts []string, methods *[]PythonMethod) {
	if c.NoPyAPI {
		return
	}

	if c.Run != nil && c.ArgParser != nil {
		c.collectSingleMethod(pythonName, cmdParts, methods)
	}

	for subCmdName, subCmd := range c.SubCommands {
		// Convert hyphens to underscores for valid Python method names
		pythonSubCmdName := strings.ReplaceAll(subCmdName, "-", "_")
		var newPythonName string
		if pythonName == "" {
			newPythonName = pythonSubCmdName
		} else {
			newPythonName = pythonName + "_" + pythonSubCmdName
		}

		newCmdParts := append([]string{}, cmdParts...)
		newCmdParts = append(newCmdParts, subCmdName)

		subCmd.collectMethods(newPythonName, newCmdParts, methods)
	}
}
