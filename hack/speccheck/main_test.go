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

package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyFixture copies a fixture tree into a temporary directory and returns its path. Files named *_test.go.txt are
// renamed to *_test.go, so that the fixtures are not compiled or scanned as part of this repository.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := os.DirFS(filepath.Join("testdata", name))
	dst := t.TempDir()
	require.NoError(t, fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, strings.TrimSuffix(filepath.FromSlash(path), ".txt"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		content, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o600)
	}))
	return dst
}

func TestCheckPassesWhenImplementedRequirementsAreClaimed(t *testing.T) {
	r, err := check(copyFixture(t, "ok"), "docs/specs")
	require.NoError(t, err)

	assert.Empty(t, r.Problems)
	require.Len(t, r.Specs, 3)
	assert.Equal(t, "docs/specs/alpha/spec.md", r.Specs[0].Path)
	assert.Equal(t, statusImplemented, r.Specs[0].Status)

	// A struck-through requirement is not a requirement any more.
	ids := []string{}
	for _, req := range r.Specs[0].Requirements {
		ids = append(ids, req.ID)
	}
	assert.Equal(t, []string{"ALPHA-001", "ALPHA-002"}, ids)

	// One test may claim several requirements, and several tests may claim one.
	assert.Len(t, r.Claims["ALPHA-001"], 2)
	assert.Len(t, r.Claims["ALPHA-002"], 1)
	assert.Equal(t, "pkg/widget_test.go", r.Claims["ALPHA-002"][0].Path)
	assert.Equal(t, 4, r.Claims["ALPHA-002"][0].Line)

	// Requirements of specifications that are not implemented yet may be unclaimed.
	assert.Empty(t, r.Claims["BETA-001"])
	assert.Empty(t, r.Claims["GAMMA-001"])
}

func TestCheckReportsEveryKindOfProblem(t *testing.T) {
	r, err := check(copyFixture(t, "bad"), "docs/specs")
	require.NoError(t, err)

	problems := strings.Join(r.Problems, "\n")
	assert.Len(t, r.Problems, 5)
	assert.Contains(t, problems,
		"docs/specs/alpha/spec.md:5: ALPHA-002 belongs to an implemented specification but no test claims it")
	assert.Contains(t, problems, `docs/specs/beta/spec.md: status "done" is not one of`)
	assert.Contains(t, problems, "docs/specs/beta/spec.md:4: ALPHA-001 is already defined in docs/specs/alpha/spec.md")
	assert.Contains(t, problems, `docs/specs/gamma/spec.md: status "" is not one of`)
	assert.Contains(t, problems, "pkg/widget_test.go:4: test claims OMEGA-001, which no specification defines")
}

func TestCheckAcceptsARepositoryWithoutSpecifications(t *testing.T) {
	r, err := check(copyFixture(t, "empty"), "docs/specs")
	require.NoError(t, err)

	assert.Empty(t, r.Specs)
	assert.Empty(t, r.Problems)
}

func TestReportIsMarkdown(t *testing.T) {
	r, err := check(copyFixture(t, "ok"), "docs/specs")
	require.NoError(t, err)

	var out bytes.Buffer
	r.write(&out)

	assert.Contains(t, out.String(), "| Specification | Status | Requirement | Tests |")
	assert.Contains(t, out.String(),
		"| `docs/specs/alpha/spec.md` | implemented | **ALPHA-001** The widget is created. | "+
			"`pkg/widget_test.go:3`<br>`pkg/widget_test.go:4` |")
	// A pipe in the requirement text must not break the table.
	assert.Contains(t, out.String(), `deleted \| cleaned up.`)
	assert.Contains(t, out.String(), "| `docs/specs/beta/spec.md` | approved | **BETA-001** Not implemented yet. | — |")
	assert.Contains(t, out.String(), "No problems found.")
}

func TestRunExitsWithOneWhenThereAreProblems(t *testing.T) {
	var stdout, stderr bytes.Buffer

	assert.Equal(t, 0, run([]string{"-root", copyFixture(t, "ok")}, &stdout, &stderr))
	assert.Equal(t, 1, run([]string{"-root", copyFixture(t, "bad")}, &stdout, &stderr))
	assert.Contains(t, stdout.String(), "### Problems (5)")
	assert.Equal(t, 2, run([]string{"-unknown-flag"}, &stdout, &stderr))
}
