/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Command crdcompat detects backward-incompatible changes to CustomResourceDefinitions.
//
// It compares the CRD manifests in two directories, the old and the new state of the API, and reports every change
// that can break existing objects or clients: removed kinds, versions, and fields, changed types, fields that became
// required, and tightened validation. Additions and loosened validation are compatible and are not reported.
//
// An incompatible change is sometimes intended. It passes when its exact description is recorded in the
// acknowledgment file, which makes breaking the API a deliberate, reviewed act rather than a side effect.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("crdcompat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	oldDir := flags.String("old", "", "directory that holds the CRD manifests before the change")
	newDir := flags.String("new", "config/crd/bases", "directory that holds the CRD manifests after the change")
	ack := flags.String("acknowledged", "", "file that records the incompatible changes that are intended")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	oldCRDs, err := loadCRDs(*oldDir)
	if err != nil {
		fmt.Fprintln(stderr, "crdcompat:", err)
		return 2
	}
	newCRDs, err := loadCRDs(*newDir)
	if err != nil {
		fmt.Fprintln(stderr, "crdcompat:", err)
		return 2
	}
	acknowledged := ""
	if *ack != "" {
		content, err := os.ReadFile(filepath.Clean(*ack))
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(stderr, "crdcompat:", err)
			return 2
		}
		acknowledged = string(content)
	}

	var pending []string
	changes := compare(oldCRDs, newCRDs)
	fmt.Fprintln(stdout, "## API compatibility")
	fmt.Fprintln(stdout)
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "No backward-incompatible changes to the CustomResourceDefinitions.")
		return 0
	}
	fmt.Fprintln(stdout, "Backward-incompatible changes to the CustomResourceDefinitions:")
	fmt.Fprintln(stdout)
	for _, change := range changes {
		state := "acknowledged"
		if !strings.Contains(acknowledged, "`"+change+"`") {
			state = "**not acknowledged**"
			pending = append(pending, change)
		}
		fmt.Fprintf(stdout, "- `%s` (%s)\n", change, state)
	}
	if len(pending) == 0 {
		return 0
	}
	fmt.Fprintln(stdout)
	if *ack != "" {
		fmt.Fprintf(stdout, "If a change is intended, record its description in backticks, with the reason, in `%s`.\n",
			*ack)
	}
	return 1
}

// loadCRDs reads every YAML manifest in dir and returns the CRDs by name. A directory that does not exist holds none.
func loadCRDs(dir string) (map[string]*apiextensionsv1.CustomResourceDefinition, error) {
	crds := map[string]*apiextensionsv1.CustomResourceDefinition{}
	if dir == "" {
		return crds, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, err
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(content, crd); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if crd.Kind != "CustomResourceDefinition" {
			continue
		}
		crds[crd.Name] = crd
	}
	return crds, nil
}

// compare returns a sorted description of every backward-incompatible difference between two sets of CRDs.
func compare(oldCRDs, newCRDs map[string]*apiextensionsv1.CustomResourceDefinition) []string {
	var changes []string
	for name, oldCRD := range oldCRDs {
		newCRD, ok := newCRDs[name]
		if !ok {
			changes = append(changes, name+": removed")
			continue
		}
		if oldCRD.Spec.Scope != newCRD.Spec.Scope {
			changes = append(changes,
				fmt.Sprintf("%s: scope changed from %s to %s", name, oldCRD.Spec.Scope, newCRD.Spec.Scope))
		}
		newVersions := map[string]apiextensionsv1.CustomResourceDefinitionVersion{}
		for _, version := range newCRD.Spec.Versions {
			newVersions[version.Name] = version
		}
		for _, oldVersion := range oldCRD.Spec.Versions {
			prefix := name + "/" + oldVersion.Name
			newVersion, ok := newVersions[oldVersion.Name]
			switch {
			case !ok:
				changes = append(changes, prefix+": removed")
			case oldVersion.Served && !newVersion.Served:
				changes = append(changes, prefix+": no longer served")
			case oldVersion.Schema != nil && newVersion.Schema != nil:
				for _, change := range compareSchema(
					oldVersion.Schema.OpenAPIV3Schema, newVersion.Schema.OpenAPIV3Schema, "") {
					changes = append(changes, prefix+": "+change)
				}
			}
		}
	}
	sort.Strings(changes)
	return changes
}

// compareSchema returns the backward-incompatible differences between two schemas. path is the JSON path of the
// schemas within the object, and is empty for the root.
func compareSchema(oldSchema, newSchema *apiextensionsv1.JSONSchemaProps, path string) []string {
	if oldSchema == nil || newSchema == nil {
		return nil
	}
	at := path
	if at == "" {
		at = "."
	}
	if oldSchema.Type != newSchema.Type {
		// The rest of the schemas describe different things, so comparing them would only add noise.
		return []string{fmt.Sprintf("%s: type changed from %s to %s",
			at, describe(oldSchema.Type), describe(newSchema.Type))}
	}
	var changes []string

	changes = append(changes, compareRequired(oldSchema, newSchema, path)...)
	changes = append(changes, compareEnum(oldSchema, newSchema, at)...)
	changes = append(changes, compareBounds(oldSchema, newSchema, at)...)

	names := make([]string, 0, len(oldSchema.Properties))
	for name := range oldSchema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		oldProperty := oldSchema.Properties[name]
		newProperty, ok := newSchema.Properties[name]
		if !ok {
			changes = append(changes, path+"."+name+": removed")
			continue
		}
		changes = append(changes, compareSchema(&oldProperty, &newProperty, path+"."+name)...)
	}

	if oldSchema.Items != nil && newSchema.Items != nil {
		changes = append(changes, compareSchema(oldSchema.Items.Schema, newSchema.Items.Schema, path+"[]")...)
	}
	if oldSchema.AdditionalProperties != nil && newSchema.AdditionalProperties != nil {
		changes = append(changes, compareSchema(
			oldSchema.AdditionalProperties.Schema, newSchema.AdditionalProperties.Schema, path+"{}")...)
	}
	return changes
}

func compareRequired(oldSchema, newSchema *apiextensionsv1.JSONSchemaProps, path string) []string {
	oldRequired := map[string]bool{}
	for _, name := range oldSchema.Required {
		oldRequired[name] = true
	}
	var changes []string
	for _, name := range newSchema.Required {
		if !oldRequired[name] {
			changes = append(changes, path+"."+name+": became required")
		}
	}
	return changes
}

func compareBounds(oldSchema, newSchema *apiextensionsv1.JSONSchemaProps, at string) []string {
	var changes []string
	report := func(format string, args ...any) {
		changes = append(changes, at+": "+fmt.Sprintf(format, args...))
	}
	switch {
	case newSchema.Minimum != nil && oldSchema.Minimum == nil:
		report("minimum %v added", *newSchema.Minimum)
	case newSchema.Minimum != nil && *newSchema.Minimum > *oldSchema.Minimum:
		report("minimum raised from %v to %v", *oldSchema.Minimum, *newSchema.Minimum)
	}
	switch {
	case newSchema.Maximum != nil && oldSchema.Maximum == nil:
		report("maximum %v added", *newSchema.Maximum)
	case newSchema.Maximum != nil && *newSchema.Maximum < *oldSchema.Maximum:
		report("maximum lowered from %v to %v", *oldSchema.Maximum, *newSchema.Maximum)
	}
	if newSchema.Pattern != "" && newSchema.Pattern != oldSchema.Pattern {
		report("pattern changed from %s to %s", describe(oldSchema.Pattern), describe(newSchema.Pattern))
	}
	return changes
}

func compareEnum(oldSchema, newSchema *apiextensionsv1.JSONSchemaProps, at string) []string {
	if len(newSchema.Enum) == 0 {
		return nil
	}
	if len(oldSchema.Enum) == 0 {
		return []string{at + ": enum restriction added"}
	}
	allowed := map[string]bool{}
	for _, value := range newSchema.Enum {
		allowed[string(value.Raw)] = true
	}
	var changes []string
	for _, value := range oldSchema.Enum {
		if !allowed[string(value.Raw)] {
			changes = append(changes, fmt.Sprintf("%s: enum value %s removed", at, value.Raw))
		}
	}
	return changes
}

func describe(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
