package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/vaikas/gofunctypechecker/pkg/detect"
)

const supportedFuncs = `
Could not find a supported function signature. Examples of supported functions are
shown below, also showing the imports that you can use. The function must also be visible
outside of the package (capitalized, for example, Receive vs. receive).

import (
    "context"
    event "github.com/cloudevents/sdk-go/v2"
    "github.com/cloudevents/sdk-go/v2/protocol"
)

The following function signatures are supported by this builder:
func(event.Event)
func(event.Event) protocol.Result
func(event.Event) error
func(context.Context, event.Event)
func(context.Context, event.Event) protocol.Result
func(context.Context, event.Event) error
func(event.Event) *event.Event
func(event.Event) (*event.Event, protocol.Result)
func(event.Event) (*event.Event, error)
func(context.Context, event.Event) *event.Event
func(context.Context, event.Event) (*event.Event, protocol.Result)
func(context.Context, event.Event) (*event.Event, error)
`

const planFileFormat = `
[[provides]]
name = "ce-go-function"
[[requires]]
name = "ce-go-function"
[requires.metadata]
package = "PACKAGE"
function = "CE_GO_FUNCTION"
protocol = "CE_PROTOCOL"
`

// import paths for supported functions
const (
	ceImport         = "github.com/cloudevents/sdk-go/v2"
	ceProtocolImport = "github.com/cloudevents/sdk-go/v2/protocol"
	contextImport    = "context"
)

type EnvConfig struct {
	CEGoPackage  string `envconfig:"CE_GO_PACKAGE" default:"./"`
	CEGoFunction string `envconfig:"CE_GO_FUNCTION" default:"Receiver"`
	CEProtocol   string `envconfig:"CE_PROTOCOL" default:"http"`
}

// Define the valid functions like so:
/*
	"func(event.Event)"
	"func(event.Event) protocol.Result"
	"func(event.Event) error"
	"func(context.Context, event.Event)"
	"func(context.Context, event.Event) protocol.Result"
	"func(context.Context, event.Event) error"
	"func(event.Event) *event.Event"
	"func(event.Event) (*event.Event, protocol.Result)"
	"func(event.Event) (*event.Event, error)"
	"func(context.Context, event.Event) *event.Event":                    functionSignature{in: []paramType{contextType, eventType}, out: []paramType{ptrEventType}},
	"func(context.Context, event.Event) (*event.Event, protocol.Result)": functionSignature{in: []paramType{contextType, eventType}, out: []paramType{ptrEventType, protocolResultType}},
	"func(context.Context, event.Event) (*event.Event, error)":           functionSignature{in: []paramType{contextType, eventType}, out: []paramType{ptrEventType, errorType}},
*/

var validFunctions = []detect.FunctionSignature{
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}},
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceProtocolImport, Name: "Result"}}},
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{Name: "error"}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceProtocolImport, Name: "Result"}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{Name: "error"}}},
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}}},
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}, {ImportPath: ceProtocolImport, Name: "Result"}}},
	{In: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}, {Name: "error"}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}, {ImportPath: ceProtocolImport, Name: "Result"}}},
	{In: []detect.FunctionArg{{ImportPath: contextImport, Name: "Context"}, {ImportPath: ceImport, Name: "Event"}}, Out: []detect.FunctionArg{{ImportPath: ceImport, Name: "Event", Pointer: true}, {Name: "error"}}},
}

func printSupportedFunctionsAndExit() {
	fmt.Println(supportedFuncs)
	os.Exit(100)
}

func main() {
	log.Println("ARGS: ", os.Args)
	for _, e := range os.Environ() {
		log.Println(e)
	}

	if len(os.Args) < 3 {
		log.Printf("Usage: %s <PLATFORM_DIR> <BUILD_PLAN>\n", os.Args[0])
		os.Exit(100)
	}

	// Grab the env variables
	var envConfig EnvConfig
	if err := envconfig.Process("http", &envConfig); err != nil {
		log.Printf("Failed to process env variables: %s\n", err)
	}

	moduleName, err := readModuleName()
	if err != nil {
		log.Println("Failed to read go.mod file: ", err)
		os.Exit(100)
	}

	// There are two ENV variables that control what should be checked.
	// We yank the base package from go.mod and append CE_GO_PACKAGE into it
	// if it's given.
	goPackage := envConfig.CEGoPackage
	if !strings.HasSuffix(goPackage, "/") {
		goPackage = goPackage + "/"
	}
	fullGoPackage := moduleName
	if goPackage != "./" {
		fullGoPackage = fullGoPackage + "/" + filepath.Clean(goPackage)
	}
	log.Println("Using relative path to look for function: ", goPackage)

	goFunction := envConfig.CEGoFunction
	goProtocol := envConfig.CEProtocol

	planFileName := os.Args[2]
	log.Println("using plan file: ", planFileName)

	// read all go files from the directory that was given. Note that if no directory (CE_PACKAGE)
	// was given, this is ./
	files, err := filepath.Glob(fmt.Sprintf("%s*.go", goPackage))
	if err != nil {
		log.Printf("failed to read directory %s : %s\n", goPackage, err)
		printSupportedFunctionsAndExit()
	}

	detector := detect.NewDetector(validFunctions)

	for _, f := range files {
		log.Printf("Processing file %s\n", f)
		// read the whole file in
		srcbuf, err := ioutil.ReadFile(f)
		if err != nil {
			log.Println(err)
			printSupportedFunctionsAndExit()
		}
		f := &detect.Function{File: f, Source: string(srcbuf)}
		deets, err := detector.CheckFile(f)
		if err != nil {
			log.Printf("Failed to check file: %q : %s\n", f, err)
			os.Exit(100)
		}
		if deets != nil {
			log.Printf("Found supported function %q in package %q signature %q", deets.Name, deets.Package, deets.Signature)
			// If the user didn't specify a specific function, use it. If they specified the function, make sure it
			// matches what we found.
			if goFunction == "" || goFunction == deets.Name {
				deets.Package = fullGoPackage
				if err := writePlan(planFileName, goProtocol, deets); err != nil {
					log.Println("failed to write the build plan: ", err)
				}
				os.Exit(0)
			}
		}
	}
	printSupportedFunctionsAndExit()
}

// writePlan writes the planFileName with the following format:
//[[provides]]
//name = "ce-go-function"
//[[requires]]
//name = "ce-go-function"
//[requires.metadata]
//package = <details.packageName>
//function = "details.Name"
//protocol = "http"
func writePlan(planFileName, protocol string, details *detect.FunctionDetails) error {
	planFile, err := os.OpenFile(planFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("failed to open the plan file for writing", os.Args[2], err)
		printSupportedFunctionsAndExit()
	}
	defer planFile.Close()

	// Replace the placeholders with valid values
	replacedPlan := strings.Replace(string(planFileFormat), "PACKAGE", details.Package, 1)
	replacedPlan = strings.Replace(replacedPlan, "CE_GO_FUNCTION", details.Name, 1)
	replacedPlan = strings.Replace(replacedPlan, "CE_PROTOCOL", protocol, 1)
	if _, err := planFile.WriteString(replacedPlan); err != nil {
		printSupportedFunctionsAndExit()
	}
	return nil
}

// readModuleName is a terrible hack for yanking the module from go.mod file.
// Should be replaced with something that actually understands go...
func readModuleName() (string, error) {
	modFile, err := os.Open("./go.mod")
	if err != nil {
		return "", err
	}
	defer modFile.Close()
	scanner := bufio.NewScanner(modFile)
	for scanner.Scan() {
		pieces := strings.Split(scanner.Text(), " ")
		fmt.Printf("Found pieces as %+v\n", pieces)
		if len(pieces) >= 2 && pieces[0] == "module" {
			return pieces[1], nil
		}
	}
	return "", nil
}
