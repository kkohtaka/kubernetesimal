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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diff = `diff --git a/pkg/a.go b/pkg/a.go
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -10,0 +11,3 @@ func a() {
+one
+two
+three
@@ -20 +23 @@ func b() {
-old
+new
@@ -30,2 +32,0 @@ func c() {
-gone
-gone
diff --git a/pkg/a_test.go b/pkg/a_test.go
--- a/pkg/a_test.go
+++ b/pkg/a_test.go
@@ -1,0 +2 @@
+test
diff --git a/pkg/zz_generated.deepcopy.go b/pkg/zz_generated.deepcopy.go
--- a/pkg/zz_generated.deepcopy.go
+++ b/pkg/zz_generated.deepcopy.go
@@ -1,0 +2 @@
+generated
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1,0 +2 @@
+docs
diff --git a/pkg/removed.go b/pkg/removed.go
--- a/pkg/removed.go
+++ /dev/null
@@ -1,3 +0,0 @@
-gone
`

const profile = `mode: set
example.com/mod/pkg/a.go:5.1,9.2 2 1
example.com/mod/pkg/a.go:11.1,12.10 3 1
example.com/mod/pkg/a.go:13.1,15.2 1 0
example.com/mod/pkg/a.go:23.1,23.20 4 0
example.com/mod/pkg/a.go:23.1,23.20 4 1
example.com/mod/pkg/a.go:40.1,42.2 5 0
example.com/mod/pkg/zz_generated.deepcopy.go:1.1,9.2 7 0
`

func TestParseDiffCollectsAddedLinesOfGoSources(t *testing.T) {
	changed, err := parseDiff(strings.NewReader(diff), regexp.MustCompile(`zz_generated`))
	require.NoError(t, err)

	// Deleted lines, test files, excluded files, and files that are not Go are ignored.
	assert.Equal(t, map[string]map[int]bool{
		"pkg/a.go": {11: true, 12: true, 13: true, 23: true},
	}, changed)
}

func TestEvaluateCountsOnlyBlocksThatOverlapChangedLines(t *testing.T) {
	changed, err := parseDiff(strings.NewReader(diff), regexp.MustCompile(`zz_generated`))
	require.NoError(t, err)
	blocks, err := parseProfile(strings.NewReader(profile), "example.com/mod")
	require.NoError(t, err)

	r := evaluate(changed, blocks)

	// Lines 11-12 are covered (3), line 13 is not (1), and line 23 is covered by one of two test binaries (4).
	// The blocks at lines 5-9 and 40-42 are not touched by the diff.
	assert.Equal(t, 8, r.Total)
	assert.Equal(t, 7, r.Covered)
	assert.Equal(t, []block{{File: "pkg/a.go", Start: 13, End: 15, Statements: 1}}, r.Uncovered)
	assert.InDelta(t, 87.5, r.percent(), 0.01)
}

func TestParseProfileRejectsMalformedLines(t *testing.T) {
	_, err := parseProfile(strings.NewReader("mode: set\nnot a block\n"), "example.com/mod")
	assert.ErrorContains(t, err, "malformed coverage profile line")
}

func TestRunComparesAgainstTheMinimum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.out")
	require.NoError(t, os.WriteFile(path, []byte(profile), 0o600))
	args := []string{"-profile", path, "-module", "example.com/mod", "-exclude", "zz_generated"}

	var stdout, stderr bytes.Buffer
	assert.Equal(t, 0, run(append(args, "-min", "80"), strings.NewReader(diff), &stdout, &stderr))
	assert.Contains(t, stdout.String(), "7 of 8 changed statements are covered by tests: 87.5% (minimum 80.0%).")
	assert.Contains(t, stdout.String(), "- `pkg/a.go:13-15`")

	assert.Equal(t, 1, run(append(args, "-min", "90"), strings.NewReader(diff), &stdout, &stderr))

	// A diff that changes no statements passes whatever the minimum is.
	stdout.Reset()
	assert.Equal(t, 0, run(append(args, "-min", "100"), strings.NewReader(""), &stdout, &stderr))
	assert.Contains(t, stdout.String(), "No changed statements to cover.")

	assert.Equal(t, 2, run([]string{"-profile", filepath.Join(t.TempDir(), "missing")},
		strings.NewReader(diff), &stdout, &stderr))
	assert.Equal(t, 2, run([]string{"-exclude", "("}, strings.NewReader(diff), &stdout, &stderr))
}
