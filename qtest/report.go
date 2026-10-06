// Copyright 2026 Magnobit, Inc. All rights reserved.

package qtest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSON renders the summary for CI.
func (s Summary) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// JUnit renders a single testsuite. One failure is one testcase.
func (s Summary) JUnit() string {
	var b strings.Builder
	total := s.Passed + s.Failed
	fmt.Fprintf(&b, "<testsuite name=\"quell\" tests=\"%d\" failures=\"%d\" seed=\"%d\">\n", total, s.Failed, s.Seed)
	for i := 0; i < s.Passed; i++ {
		b.WriteString("  <testcase name=\"passed\"/>\n")
	}
	for _, f := range s.Failures {
		fmt.Fprintf(&b, "  <testcase name=\"%s\" file=\"%s\" line=\"%d\">\n", xmlEscape(f.Assertion), xmlEscape(f.File), f.Line)
		fmt.Fprintf(&b, "    <failure message=\"expected %s\">actual %s</failure>\n", xmlEscape(f.Expected), xmlEscape(f.Actual))
		b.WriteString("  </testcase>\n")
	}
	b.WriteString("</testsuite>\n")
	return b.String()
}

// Text is the project summary, including the seed.
func (s Summary) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "quell test  passed=%d failed=%d seed=%d\n", s.Passed, s.Failed, s.Seed)
	for _, f := range s.Failures {
		fmt.Fprintf(&b, "%s:%d %s\n  expected %s\n  actual   %s\n", f.File, f.Line, f.Assertion, f.Expected, f.Actual)
	}
	return b.String()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
