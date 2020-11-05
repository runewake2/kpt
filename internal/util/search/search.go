package search

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/GoogleContainerTools/kpt/pkg/kptfile"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const PathDelimiter = "."

// SearchReplace struct holds the input parameters and results for
// Search and Replace operations on resource configs
type SearchReplace struct {
	// ByValue is the value of the field to be matched
	ByValue string

	// ByValueRegex is the value regex of the field to be matched
	ByValueRegex string

	regex *regexp.Regexp

	// ByPath is the path of the field to be matched
	ByPath string

	// Count is the number of matches
	Count int

	// Match is the map of file path to list of matched fields
	Match map[string][]string

	// PutLiteral is the literal to be put at to field
	// filtered by path and/or value
	PutLiteral string

	filePath string
}

// Perform performs the search and replace operation on each node in the package path
func (sr *SearchReplace) Perform(resourcesPath string) error {
	inout := &kio.LocalPackageReadWriter{
		PackagePath:     resourcesPath,
		NoDeleteFiles:   true,
		PackageFileName: kptfile.KptFileName,
	}

	sr.Match = make(map[string][]string)

	if sr.ByValueRegex != "" {
		re, err := regexp.Compile(sr.ByValueRegex)
		if err != nil {
			return errors.Wrap(err)
		}
		sr.regex = re
	}

	return kio.Pipeline{
		Inputs:  []kio.Reader{inout},
		Filters: []kio.Filter{kio.FilterAll(sr)},
		Outputs: []kio.Writer{inout},
	}.Execute()
}

// Filter parses input node and performs search and replace operation on the node
func (sr *SearchReplace) Filter(object *yaml.RNode) (*yaml.RNode, error) {
	filePath, _, err := kioutil.GetFileAnnotations(object)
	if err != nil {
		return object, err
	}
	sr.filePath = filePath

	if sr.shouldPutLiteralByPath() {
		return object, sr.putLiteral(object)
	}

	// traverse the node to perform search/put operation
	err = accept(sr, object)
	return object, err
}

// visitMapping parses mapping node
func (sr *SearchReplace) visitMapping(object *yaml.RNode, path string) error {
	return nil
}

// visitSequence parses sequence node
func (sr *SearchReplace) visitSequence(object *yaml.RNode, path string) error {
	// TODO: pmarupaka support sequence nodes
	return nil
}

// visitScalar parses scalar node
func (sr *SearchReplace) visitScalar(object *yaml.RNode, path string) error {
	pathMatch, err := sr.pathMatch(path)
	if err != nil {
		return err
	}

	valueMatch := object.Document().Value == sr.ByValue || sr.regexMatch(object.Document().Value)

	// at least one of path or value must be matched
	if (valueMatch && pathMatch) || (valueMatch && sr.ByPath == "") ||
		(pathMatch && sr.ByValue == "" && sr.ByValueRegex == "") {
		sr.Count++
		if sr.PutLiteral != "" {
			// TODO: pmarupaka Check if the new value honors the openAPI schema and/or
			// current field type, throw error if it doesn't
			object.Document().Value = sr.PutLiteral
		}
		if sr.filePath != "" {
			pathVal := fmt.Sprintf("%s: %s", strings.TrimPrefix(path, PathDelimiter), object.Document().Value)
			sr.Match[sr.filePath] = append(sr.Match[sr.filePath], pathVal)
		}
	}
	return nil
}

// regexMatch checks if ValueRegex in SearchReplace struct matches with the input
// value, returns error if any
func (sr *SearchReplace) regexMatch(value string) bool {
	if sr.ByValueRegex == "" {
		return false
	}
	return sr.regex.Match([]byte(value))
}

// pathMatch checks if the traversed yaml path matches with the user input path
// checks if user input path is valid
func (sr *SearchReplace) pathMatch(yamlPath string) (bool, error) {
	if sr.ByPath == "" {
		return false, nil
	}
	// TODO: pmarupaka Path expressions should be supported
	if !isAbsPath(sr.ByPath) {
		return false, errors.Errorf(`invalid input path, must follow pattern e.g. foo.bar.baz`)
	}
	return sr.ByPath == strings.TrimPrefix(yamlPath, PathDelimiter), nil
}

// isAbsPath checks if input path is absolute and not a path expression
// only supported path format is e.g. foo.bar.baz
func isAbsPath(path string) bool {
	pathElem := strings.Split(path, PathDelimiter)
	if len(pathElem) == 0 {
		return false
	}
	for _, elem := range pathElem {
		// more checks can be added in future
		if elem == "" || strings.Contains(elem, "*") {
			return false
		}
	}
	return true
}

// putLiteral puts the literal in the user specified sr.ByPath
func (sr *SearchReplace) putLiteral(object *yaml.RNode) error {
	path := strings.Split(sr.ByPath, PathDelimiter)
	// lookup(or create) node for n-1 path elements
	node, err := object.Pipe(yaml.LookupCreate(yaml.MappingNode, path[:len(path)-1]...))
	if err != nil {
		return errors.Wrap(err)
	}
	// set the last path element key with the input value
	err = node.PipeE(yaml.SetField(path[len(path)-1], yaml.NewScalarRNode(sr.PutLiteral)))
	if err != nil {
		return errors.Wrap(err)
	}
	sr.Match[sr.filePath] = append(sr.Match[sr.filePath], fmt.Sprintf("%s: %s", sr.ByPath, sr.PutLiteral))
	sr.Count++
	return nil
}

// shouldPutLiteralByPath returns true if only absolute path and literal are provided,
// so that the value can be directly put without needing to traverse the entire node,
// handles the case of adding non-existent field-value to node
func (sr *SearchReplace) shouldPutLiteralByPath() bool {
	return isAbsPath(sr.ByPath) && sr.ByValue == "" && sr.ByValueRegex == "" && sr.PutLiteral != ""
}