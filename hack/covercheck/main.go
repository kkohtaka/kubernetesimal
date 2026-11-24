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

// Command covercheck verifies that changed code is covered by tests.
//
// It reads a unified diff from standard input, as produced by "git diff -U0", and a Go coverage profile. Every
// statement that overlaps a line added or changed by the diff has to be executed by at least one test, up to the
// given minimum percentage. Code that the diff does not touch is not judged, so that code without tests does not block
// unrelated work but cannot grow.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	hunkPattern  = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	blockPattern = regexp.MustCompile(`^(.+):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$`)
)

// block is one basic block of a coverage profile.
type block struct {
	File       string
	Start, End int
	Statements int
	Covered    bool
}

// result is the coverage of the statements touched by a diff.
type result struct {
	Total     int
	Covered   int
	Uncovered []block
}

func (r result) percent() float64 {
	if r.Total == 0 {
		return 100
	}
	return float64(r.Covered) * 100 / float64(r.Total)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("covercheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profile := flags.String("profile", "cover.out", "coverage profile written by go test -coverprofile")
	module := flags.String("module", "", "module path, which prefixes the file names in the profile")
	minimum := flags.Float64("min", 80, "minimum percentage of changed statements that tests must cover")
	exclude := flags.String("exclude", "", "regular expression of file paths that are not judged")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	var excluded *regexp.Regexp
	if *exclude != "" {
		var err error
		if excluded, err = regexp.Compile(*exclude); err != nil {
			fmt.Fprintln(stderr, "covercheck:", err)
			return 2
		}
	}
	changed, err := parseDiff(stdin, excluded)
	if err != nil {
		fmt.Fprintln(stderr, "covercheck:", err)
		return 2
	}
	f, err := os.Open(filepath.Clean(*profile))
	if err != nil {
		fmt.Fprintln(stderr, "covercheck:", err)
		return 2
	}
	defer func() { _ = f.Close() }()
	blocks, err := parseProfile(f, *module)
	if err != nil {
		fmt.Fprintln(stderr, "covercheck:", err)
		return 2
	}

	r := evaluate(changed, blocks)
	r.write(stdout, *minimum)
	if r.percent() < *minimum {
		return 1
	}
	return 0
}

// parseDiff returns the added and changed line numbers of every Go source file in a unified diff. Test files and
// files that match excluded are left out.
func parseDiff(r io.Reader, excluded *regexp.Regexp) (map[string]map[int]bool, error) {
	changed := map[string]map[int]bool{}
	file := ""
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if path, ok := strings.CutPrefix(line, "+++ "); ok {
			file = strings.TrimPrefix(path, "b/")
			if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") ||
				(excluded != nil && excluded.MatchString(file)) {
				file = ""
			}
			continue
		}
		m := hunkPattern.FindStringSubmatch(line)
		if m == nil || file == "" {
			continue
		}
		start, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, err
		}
		count := 1
		if m[2] != "" {
			if count, err = strconv.Atoi(m[2]); err != nil {
				return nil, err
			}
		}
		for i := start; i < start+count; i++ {
			if changed[file] == nil {
				changed[file] = map[int]bool{}
			}
			changed[file][i] = true
		}
	}
	return changed, scanner.Err()
}

// parseProfile reads a coverage profile. A block that appears more than once, which happens when several test
// binaries cover the same package, is covered when any of its occurrences is.
func parseProfile(r io.Reader, module string) ([]block, error) {
	type key struct {
		file       string
		start, end int
	}
	merged := map[key]*block{}
	var order []key
	prefix := strings.TrimSuffix(module, "/") + "/"
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || strings.TrimSpace(line) == "" {
			continue
		}
		m := blockPattern.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("malformed coverage profile line: %q", line)
		}
		numbers := make([]int, 0, 4)
		for _, field := range m[2:] {
			n, err := strconv.Atoi(field)
			if err != nil {
				return nil, err
			}
			numbers = append(numbers, n)
		}
		k := key{file: strings.TrimPrefix(m[1], prefix), start: numbers[0], end: numbers[1]}
		b, ok := merged[k]
		if !ok {
			b = &block{File: k.file, Start: k.start, End: k.end, Statements: numbers[2]}
			merged[k] = b
			order = append(order, k)
		}
		if numbers[3] > 0 {
			b.Covered = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	blocks := make([]block, 0, len(order))
	for _, k := range order {
		blocks = append(blocks, *merged[k])
	}
	return blocks, nil
}

// evaluate counts the statements of the blocks that overlap a changed line.
func evaluate(changed map[string]map[int]bool, blocks []block) result {
	var r result
	for _, b := range blocks {
		lines := changed[b.File]
		touched := false
		for line := b.Start; line <= b.End && !touched; line++ {
			touched = lines[line]
		}
		if !touched {
			continue
		}
		r.Total += b.Statements
		if b.Covered {
			r.Covered += b.Statements
		} else {
			r.Uncovered = append(r.Uncovered, b)
		}
	}
	sort.Slice(r.Uncovered, func(i, j int) bool {
		if r.Uncovered[i].File != r.Uncovered[j].File {
			return r.Uncovered[i].File < r.Uncovered[j].File
		}
		return r.Uncovered[i].Start < r.Uncovered[j].Start
	})
	return r
}

func (r result) write(w io.Writer, minimum float64) {
	fmt.Fprintln(w, "## Coverage of changed code")
	fmt.Fprintln(w)
	if r.Total == 0 {
		fmt.Fprintln(w, "No changed statements to cover.")
		return
	}
	fmt.Fprintf(w, "%d of %d changed statements are covered by tests: %.1f%% (minimum %.1f%%).\n",
		r.Covered, r.Total, r.percent(), minimum)
	if len(r.Uncovered) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Changed code that no test executes:")
	fmt.Fprintln(w)
	for _, b := range r.Uncovered {
		fmt.Fprintf(w, "- `%s:%d-%d`\n", b.File, b.Start, b.End)
	}
}
