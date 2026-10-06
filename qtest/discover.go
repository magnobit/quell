// Copyright 2026 Magnobit, Inc. All rights reserved.

package qtest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Case is one discovered program and its assertion file.
type Case struct {
	Source string
	Expect string
}

// RunOptions selects cases and records the seed the runner would pass to a sampled assertion.
// Exact statevector assertions do not consume the seed. It is still reported.
type RunOptions struct {
	Filter string
	Seed   int64
}

// Failure is one assertion that did not hold.
type Failure struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
}

// Summary is the discovery result. Paid providers are never selected here.
type Summary struct {
	Passed   int       `json:"passed"`
	Failed   int       `json:"failed"`
	Seed     int64     `json:"seed"`
	Notes    []string  `json:"notes,omitempty"`
	Failures []Failure `json:"failures,omitempty"`
}

// Discover finds *.quell files that have a sibling .expect file.
// Source roots are sorted by filepath.Walk, which is lexical.
func Discover(root string) ([]Case, error) {
	var out []Case
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".quell") {
			return err
		}
		expect := strings.TrimSuffix(path, ".quell") + ".expect"
		if _, err := os.Stat(expect); err != nil {
			return nil
		}
		out = append(out, Case{Source: path, Expect: expect})
		return nil
	})
	return out, err
}

// RunFile executes one case on the local simulator.
func RunFile(c Case) error {
	src, err := os.ReadFile(c.Source)
	if err != nil {
		return err
	}
	f, err := os.Open(c.Expect)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if err := runExpect(string(src), fields); err != nil {
			return &Failure{File: c.Source, Line: lineNo, Assertion: firstField(fields), Expected: expectOf(fields), Actual: err.Error()}
		}
	}
	return sc.Err()
}

func firstField(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func expectOf(fields []string) string {
	if len(fields) < 2 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

func (f *Failure) Error() string {
	return fmt.Sprintf("%s:%d: %s expected %s, actual %s", f.File, f.Line, f.Assertion, f.Expected, f.Actual)
}

func runExpect(src string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "probability":
		if len(fields) != 4 {
			return fmt.Errorf("probability bits want tol")
		}
		want, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return err
		}
		tol, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return err
		}
		return ExpectProbability(src, fields[1], want, tol)
	case "capability":
		if len(fields) != 3 {
			return fmt.Errorf("capability name status")
		}
		return ExpectProviderCapability(fields[1], fields[2])
	default:
		return fmt.Errorf("unknown assertion %s", fields[0])
	}
}

// RunDir discovers cases and returns a summary. The simulator stays local.
func RunDir(root string) (Summary, error) {
	return RunDirOptions(root, RunOptions{Seed: 1})
}

// RunDirOptions applies a filename filter. The seed is reported with the summary.
func RunDirOptions(root string, opt RunOptions) (Summary, error) {
	if opt.Seed == 0 {
		opt.Seed = 1
	}
	cases, err := Discover(root)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Seed: opt.Seed}
	for _, c := range cases {
		if opt.Filter != "" && !strings.Contains(filepath.Base(c.Source), opt.Filter) {
			continue
		}
		if err := RunFile(c); err != nil {
			sum.Failed++
			if fail, ok := err.(*Failure); ok {
				sum.Failures = append(sum.Failures, *fail)
				sum.Notes = append(sum.Notes, fail.Error())
			} else {
				sum.Notes = append(sum.Notes, err.Error())
			}
			continue
		}
		sum.Passed++
	}
	return sum, nil
}
