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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expected = []string{
	"gadgets.example.com: removed",
	"widgets.example.com/v1alpha0: removed",
	"widgets.example.com/v1alpha1: .spec.color: enum value \"green\" removed",
	"widgets.example.com/v1alpha1: .spec.labels{}: type changed from string to integer",
	"widgets.example.com/v1alpha1: .spec.legacy: removed",
	"widgets.example.com/v1alpha1: .spec.name: became required",
	"widgets.example.com/v1alpha1: .spec.name: pattern changed from (none) to ^[a-z]+$",
	"widgets.example.com/v1alpha1: .spec.parts[].id: enum restriction added",
	"widgets.example.com/v1alpha1: .spec.parts[].note: removed",
	"widgets.example.com/v1alpha1: .spec.size: maximum lowered from 10 to 5",
	"widgets.example.com/v1alpha1: .spec.size: minimum raised from 0 to 1",
	"widgets.example.com/v1alpha1: .spec.weight: type changed from integer to string",
	"widgets.example.com: scope changed from Namespaced to Cluster",
}

func TestCompareReportsIncompatibleChangesOnly(t *testing.T) {
	oldCRDs, err := loadCRDs(filepath.Join("testdata", "old"))
	require.NoError(t, err)
	newCRDs, err := loadCRDs(filepath.Join("testdata", "new"))
	require.NoError(t, err)

	// The added field .spec.added and the added enum value "blue" are compatible and are not reported.
	assert.Equal(t, expected, compare(oldCRDs, newCRDs))
}

func TestCompareAcceptsAdditions(t *testing.T) {
	oldCRDs, err := loadCRDs(filepath.Join("testdata", "old"))
	require.NoError(t, err)

	assert.Empty(t, compare(oldCRDs, oldCRDs))
	// Going from the new state back to the old one loosens the validation of size, name, and parts, makes name
	// optional, and adds kinds, versions, and fields. None of that is reported. What remains is what the new state
	// added or changed in place.
	newCRDs, err := loadCRDs(filepath.Join("testdata", "new"))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"widgets.example.com/v1alpha1: .spec.added: removed",
		"widgets.example.com/v1alpha1: .spec.color: enum value \"blue\" removed",
		"widgets.example.com/v1alpha1: .spec.labels{}: type changed from integer to string",
		"widgets.example.com/v1alpha1: .spec.weight: type changed from string to integer",
		"widgets.example.com: scope changed from Cluster to Namespaced",
	}, compare(newCRDs, oldCRDs))
	// Without an old state, nothing can break.
	assert.Empty(t, compare(nil, oldCRDs))
}

func TestRunFailsUntilEveryChangeIsAcknowledged(t *testing.T) {
	args := []string{"-old", filepath.Join("testdata", "old"), "-new", filepath.Join("testdata", "new")}
	ack := filepath.Join(t.TempDir(), "breaking-changes.md")

	var stdout, stderr bytes.Buffer
	assert.Equal(t, 1, run(append(args, "-acknowledged", ack), &stdout, &stderr))
	assert.Contains(t, stdout.String(), "- `gadgets.example.com: removed` (**not acknowledged**)")

	content := ""
	for _, change := range expected[1:] {
		content += "- `" + change + "`: intended.\n"
	}
	require.NoError(t, os.WriteFile(ack, []byte(content), 0o600))
	stdout.Reset()
	assert.Equal(t, 1, run(append(args, "-acknowledged", ack), &stdout, &stderr))
	assert.Contains(t, stdout.String(), "- `gadgets.example.com: removed` (**not acknowledged**)")
	assert.Contains(t, stdout.String(), "- `widgets.example.com/v1alpha0: removed` (acknowledged)")

	require.NoError(t, os.WriteFile(ack, []byte(content+"- `"+expected[0]+"`: intended.\n"), 0o600))
	assert.Equal(t, 0, run(append(args, "-acknowledged", ack), &stdout, &stderr))
}

func TestRunPassesWhenNothingChanged(t *testing.T) {
	dir := filepath.Join("testdata", "old")

	var stdout, stderr bytes.Buffer
	assert.Equal(t, 0, run([]string{"-old", dir, "-new", dir}, &stdout, &stderr))
	assert.Contains(t, stdout.String(), "No backward-incompatible changes")
	// A base revision without any CRD has nothing that could break.
	assert.Equal(t, 0, run([]string{"-old", filepath.Join(t.TempDir(), "missing"), "-new", dir}, &stdout, &stderr))
	assert.Equal(t, 2, run([]string{"-unknown-flag"}, &stdout, &stderr))
}
