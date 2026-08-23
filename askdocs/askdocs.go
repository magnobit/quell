// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package askdocs answers a question by keyword search over local doc
// sources — no external API, no network call. It's the same scoring
// approach qubitlabs-platform's /api/v1/ai/ask endpoint falls back to when
// ANTHROPIC_API_KEY is unset, ported here for the CLI's `quell ask`.
package askdocs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type docSection struct {
	header  string
	content string
	file    string
}

// Answer returns the best local match for question across sources (a map of
// source name, e.g. "README.md", to its raw markdown content).
func Answer(question string, sources map[string]string) string {
	var sections []docSection
	for name, content := range sources {
		sections = append(sections, splitSections(content, name)...)
	}
	return search(sections, question)
}

// splitSections breaks a markdown file into per-heading sections.
// Skips `#` lines inside code fences so shell comments don't become fake headers.
func splitSections(content, filename string) []docSection {
	var out []docSection
	var currentHeader, currentContent string
	inCode := false

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
		}
		isHeading := !inCode && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# "))
		if isHeading {
			if currentContent != "" {
				out = append(out, docSection{
					header:  currentHeader,
					content: strings.TrimSpace(currentContent),
					file:    filename,
				})
			}
			currentHeader = strings.TrimLeft(line, "# ")
			currentContent = line + "\n"
		} else {
			currentContent += line + "\n"
		}
	}
	if currentContent != "" {
		out = append(out, docSection{
			header:  currentHeader,
			content: strings.TrimSpace(currentContent),
			file:    filename,
		})
	}
	return out
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true,
	"i": true, "do": true, "what": true, "why": true, "can": true,
	"in": true, "to": true, "of": true, "and": true, "or": true, "for": true,
	"it": true, "this": true, "that": true, "with": true, "on": true, "at": true,
	"me": true, "my": true, "we": true, "us": true, "you": true, "get": true,
	"show": true, "give": true, "tell": true, "explain": true,
}

func normalize(s string) string {
	s = strings.ToLower(s)
	// strip apostrophes so "grover's" == "grovers"
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "’", "") // curly apostrophe
	return s
}

func tokenize(s string) []string {
	s = normalize(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	})
	var out []string
	for _, w := range words {
		if len(w) >= 2 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

func search(sections []docSection, question string) string {
	words := tokenize(question)
	if len(words) == 0 {
		return fallback()
	}

	ranked := rankSections(sections, words)
	if len(ranked) == 0 {
		return fallback()
	}

	best := ranked[0]
	snippet := bestSnippet(best.content, words)

	var sb strings.Builder
	fmt.Fprintf(&sb, "**%s**\n\n%s", best.header, snippet)
	return sb.String()
}

// rankSections scores each section by keyword overlap with words (content
// matches count 1, header matches count an extra 2) and returns the
// sections with score > 0, best first.
func rankSections(sections []docSection, words []string) []docSection {
	wantsCode := false
	for _, w := range words {
		switch w {
		case "example", "code", "syntax", "write", "circuit", "implement", "how":
			wantsCode = true
		}
	}

	type scored struct {
		s     docSection
		score int
	}
	var results []scored

	for _, s := range sections {
		normContent := normalize(s.content)
		normHeader := normalize(s.header)
		score := 0
		for _, w := range words {
			if strings.Contains(normContent, w) {
				score++
				if strings.Contains(normHeader, w) {
					score += 2
				}
			}
		}
		if score > 0 {
			results = append(results, scored{s, score})
		}
	}

	if len(results) == 0 {
		return nil
	}

	sort.SliceStable(results, func(i, j int) bool {
		// Sections with usable text always beat empty/header-only ones
		iText := cleanText(results[i].s.content) != ""
		jText := cleanText(results[j].s.content) != ""
		if iText != jText {
			return iText
		}
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		// On tie: prefer code sections when question asks for code
		if wantsCode {
			iCode := strings.Contains(results[i].s.content, "```")
			jCode := strings.Contains(results[j].s.content, "```")
			if iCode != jCode {
				return iCode
			}
		}
		return len(results[i].s.content) < len(results[j].s.content)
	})

	out := make([]docSection, len(results))
	for i, r := range results {
		out[i] = r.s
	}
	return out
}

// bestSnippet extracts a concise answer from a section.
// It scores paragraphs by keyword overlap but falls back to the first
// meaningful prose paragraph when scores are all zero (keyword was in the header).
func bestSnippet(content string, words []string) string {
	paras := strings.Split(content, "\n\n")

	type sp struct {
		raw    string
		clean  string
		score  int
		isCode bool
	}
	var items []sp
	var firstCode string

	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		isCode := strings.HasPrefix(p, "```")
		if isCode && firstCode == "" {
			firstCode = p
		}
		cleaned := cleanText(p)
		if cleaned == "" {
			continue
		}
		norm := normalize(p)
		score := 0
		for _, w := range words {
			if strings.Contains(norm, w) {
				score++
			}
		}
		items = append(items, sp{p, cleaned, score, isCode})
	}

	// Find best non-code paragraph by score; fall back to first prose paragraph
	var answer string
	bestScore := -1
	for _, item := range items {
		if item.isCode {
			continue
		}
		if item.score > bestScore {
			bestScore = item.score
			answer = item.clean
		}
	}
	// If every score was 0, just use the first prose paragraph
	if answer == "" || bestScore == 0 {
		for _, item := range items {
			if !item.isCode && item.clean != "" {
				answer = item.clean
				break
			}
		}
	}

	if len(answer) > 350 {
		answer = answer[:350] + "…"
	}

	// Include code only when the question asks for it
	wantsCode := false
	for _, w := range words {
		switch w {
		case "example", "code", "syntax", "write", "circuit", "implement", "how":
			wantsCode = true
		}
	}
	if wantsCode && firstCode != "" {
		return answer + "\n\n" + firstCode
	}
	return answer
}

// cleanText strips markdown headers, table rows, and bold markers,
// returning plain readable text.
var boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)

func cleanText(p string) string {
	var lines []string
	for _, l := range strings.Split(p, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "|") || l == "---" {
			continue
		}
		l = boldRe.ReplaceAllString(l, "$1")
		l = strings.TrimPrefix(l, "- ")
		l = strings.TrimPrefix(l, "* ")
		lines = append(lines, l)
	}
	return strings.Join(lines, " ")
}

func fallback() string {
	return "I couldn't find a matching answer in the docs. Try asking about:\n" +
		"• Quell syntax or gates (e.g. \"how do I write a Bell pair?\")\n" +
		"• Algorithms (e.g. \"how does Grover's algorithm work?\")\n" +
		"• Backends (e.g. \"how do I connect to IBM Quantum?\")\n" +
		"• Comparison (e.g. \"how is Quell different from Qiskit?\")"
}
