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

// Command speccheck verifies that the requirements in docs/specs are traceable to tests.
//
// A requirement is a list item in docs/specs/<name>/spec.md that starts with a bold identifier, such as
// "- **ETCD-012**: ...". A test claims a requirement by putting the identifier in square brackets in its name, such as
// It("[ETCD-012] adds members one at a time", ...).
//
// The check fails when an identifier is defined twice, when a test claims an identifier that no specification
// defines, when a specification has no valid status, or when a requirement of an implemented specification is not
// claimed by any test. It prints the traceability table as Markdown.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	statusDraft       = "draft"
	statusApproved    = "approved"
	statusImplemented = "implemented"
)

var (
	requirementPattern = regexp.MustCompile(`^\s*[-*]\s+\*\*([A-Z][A-Z0-9]*-[0-9]{3})\*\*:?\s*(.*)$`)
	claimPattern       = regexp.MustCompile(`\[([A-Z][A-Z0-9]*-[0-9]{3})\]`)
	skippedDirs        = map[string]bool{".git": true, "bin": true, "testbin": true, "testdata": true, "vendor": true}
)

// requirement is one requirement defined by a specification.
type requirement struct {
	ID   string
	Text string
	Line int
}

// spec is one parsed specification file.
type spec struct {
	Path         string
	Status       string
	Requirements []requirement
}

// claim is one place where a test names a requirement.
type claim struct {
	ID   string
	Path string
	Line int
}

// report is the result of checking specifications against tests.
type report struct {
	Specs    []spec
	Claims   map[string][]claim
	Problems []string
}

func main() {
	root := flag.String("root", ".", "root directory of the repository")
	specs := flag.String("specs", "docs/specs", "directory that holds the specifications, relative to the root")
	flag.Parse()

	r, err := check(*root, *specs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "speccheck:", err)
		os.Exit(2)
	}
	r.write(os.Stdout)
	if len(r.Problems) > 0 {
		os.Exit(1)
	}
}

func check(root, specsDir string) (*report, error) {
	specs, err := loadSpecs(root, specsDir)
	if err != nil {
		return nil, err
	}
	claims, err := loadClaims(root)
	if err != nil {
		return nil, err
	}

	r := &report{Specs: specs, Claims: map[string][]claim{}}
	definedIn := map[string]string{}
	for _, s := range specs {
		switch s.Status {
		case statusDraft, statusApproved, statusImplemented:
		default:
			r.problemf("%s: status %q is not one of %s, %s, %s",
				s.Path, s.Status, statusDraft, statusApproved, statusImplemented)
		}
		for _, req := range s.Requirements {
			if prev, ok := definedIn[req.ID]; ok {
				r.problemf("%s:%d: %s is already defined in %s", s.Path, req.Line, req.ID, prev)
				continue
			}
			definedIn[req.ID] = s.Path
		}
	}
	for _, c := range claims {
		if _, ok := definedIn[c.ID]; !ok {
			r.problemf("%s:%d: test claims %s, which no specification defines", c.Path, c.Line, c.ID)
			continue
		}
		r.Claims[c.ID] = append(r.Claims[c.ID], c)
	}
	for _, s := range specs {
		if s.Status != statusImplemented {
			continue
		}
		for _, req := range s.Requirements {
			if len(r.Claims[req.ID]) == 0 {
				r.problemf("%s:%d: %s belongs to an implemented specification but no test claims it",
					s.Path, req.Line, req.ID)
			}
		}
	}
	return r, nil
}

func (r *report) problemf(format string, args ...any) {
	r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
}

func (r *report) write(w io.Writer) {
	fmt.Fprintln(w, "## Specification traceability")
	fmt.Fprintln(w)
	if len(r.Specs) == 0 {
		fmt.Fprintln(w, "No specifications found.")
	} else {
		fmt.Fprintln(w, "| Specification | Status | Requirement | Tests |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, s := range r.Specs {
			for _, req := range s.Requirements {
				tests := "—"
				if cs := r.Claims[req.ID]; len(cs) > 0 {
					locations := make([]string, 0, len(cs))
					for _, c := range cs {
						locations = append(locations, fmt.Sprintf("`%s:%d`", c.Path, c.Line))
					}
					tests = strings.Join(locations, "<br>")
				}
				fmt.Fprintf(w, "| `%s` | %s | **%s** %s | %s |\n",
					s.Path, s.Status, req.ID, strings.ReplaceAll(req.Text, "|", `\|`), tests)
			}
		}
	}
	fmt.Fprintln(w)
	if len(r.Problems) == 0 {
		fmt.Fprintln(w, "No problems found.")
		return
	}
	fmt.Fprintf(w, "### Problems (%d)\n\n", len(r.Problems))
	for _, p := range r.Problems {
		fmt.Fprintf(w, "- %s\n", p)
	}
}

// loadSpecs parses every <specsDir>/<name>/spec.md under root. Paths in the result are relative to root.
func loadSpecs(root, specsDir string) ([]spec, error) {
	paths, err := filepath.Glob(filepath.Join(root, specsDir, "*", "spec.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	specs := make([]spec, 0, len(paths))
	for _, path := range paths {
		s, err := parseSpec(path)
		if err != nil {
			return nil, err
		}
		if s.Path, err = filepath.Rel(root, path); err != nil {
			return nil, err
		}
		s.Path = filepath.ToSlash(s.Path)
		specs = append(specs, s)
	}
	return specs, nil
}

func parseSpec(path string) (spec, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return spec{}, err
	}
	defer func() { _ = f.Close() }()

	var s spec
	inFrontMatter := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if strings.TrimSpace(text) == "---" && (line == 1 || inFrontMatter) {
			inFrontMatter = !inFrontMatter
			continue
		}
		if inFrontMatter {
			if key, value, ok := strings.Cut(text, ":"); ok && strings.TrimSpace(key) == "status" {
				s.Status = strings.TrimSpace(value)
			}
			continue
		}
		if m := requirementPattern.FindStringSubmatch(text); m != nil {
			s.Requirements = append(s.Requirements, requirement{ID: m[1], Text: m[2], Line: line})
		}
	}
	return s, scanner.Err()
}

// loadClaims finds every requirement identifier in square brackets in the Go test files under root.
func loadClaims(root string) ([]claim, error) {
	var claims []claim
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found, err := scanClaims(path, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		claims = append(claims, found...)
		return nil
	})
	return claims, err
}

func scanClaims(path, rel string) ([]claim, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var claims []claim
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		for _, m := range claimPattern.FindAllStringSubmatch(scanner.Text(), -1) {
			claims = append(claims, claim{ID: m[1], Path: rel, Line: line})
		}
	}
	return claims, scanner.Err()
}
