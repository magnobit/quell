// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/internal/parser"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println("quell " + version)
	case "help", "--help", "-h":
		printHelp()
	case "run":
		cmdRun()
	case "compile":
		cmdCompile()
	case "ask":
		cmdAsk()
	case "convert":
		cmdConvert()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\nRun 'quell help'\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdRun() {
	if len(os.Args) < 3 {
		fatalf("usage: quell run <file.quell>")
	}
	src := readFile(os.Args[2])
	circ, err := parser.Parse(src)
	must(err, "parse error")

	cfg := loadConfig()
	fmt.Printf("Backend : %s\n", cfg.Backend)
	fmt.Printf("Qubits  : %d\n", circ.NumQubits)
	fmt.Printf("Gates   : %d\n\n", len(circ.Instructions))

	switch cfg.Backend {
	case "local", "":
		out, err := compiler.Compile(circ, compiler.TargetOpenQASM)
		must(err, "compile error")
		fmt.Println("--- OpenQASM 3 ---")
		fmt.Println(out)
		fmt.Println("Tip: use QubitLabs playground for browser simulation →")
		fmt.Println("     https://qubitlabs.magnobit.com")

	case "ibm":
		qasm3, err := compiler.Compile(circ, compiler.TargetOpenQASM)
		must(err, "compile error")
		fmt.Println("  Compiled to OpenQASM 3, submitting to IBM Quantum…")
		result, err := backends.RunIBM(&cfg.IBM, qasm3, circ.NumQubits)
		must(err, "IBM run error")
		result.Print()

	case "aws":
		qasm3, err := compiler.Compile(circ, compiler.TargetOpenQASM)
		must(err, "compile error")
		fmt.Println("  Compiled to OpenQASM 3, submitting to AWS Braket…")
		result, err := backends.RunBraket(&cfg.AWS, qasm3)
		must(err, "Braket run error")
		result.Print()

	case "google":
		qasm3, err := compiler.Compile(circ, compiler.TargetOpenQASM)
		must(err, "compile error")
		fmt.Println("  Compiled to OpenQASM 3, submitting to Google Quantum Engine…")
		result, err := backends.RunGoogle(&cfg.Google, qasm3)
		must(err, "Google run error")
		result.Print()

	default:
		fatalf("unknown backend %q — valid options: local, ibm, aws, google", cfg.Backend)
	}
}

func cmdCompile() {
	args := os.Args[2:]
	target := compiler.TargetOpenQASM
	outFile := ""
	var inputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			i++
			target = compiler.Target(args[i])
		case "--output", "-o":
			i++
			outFile = args[i]
		default:
			inputFile = args[i]
		}
	}

	if inputFile == "" {
		fatalf("usage: quell compile [--target openqasm|qiskit|cirq|braket] [--output file] <file.quell>")
	}

	src := readFile(inputFile)
	circ, err := parser.Parse(src)
	must(err, "parse error")

	out, err := compiler.Compile(circ, target)
	must(err, "compile error")

	if outFile != "" {
		must(os.WriteFile(outFile, []byte(out), 0644), "write error")
		fmt.Printf("Written to %s\n", outFile)
	} else {
		fmt.Println(out)
	}
}

func cmdAsk() {
	if len(os.Args) < 3 {
		fatalf("usage: quell ask \"<question>\"")
	}
	question := strings.Join(os.Args[2:], " ")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fatalf("ANTHROPIC_API_KEY not set — run: export ANTHROPIC_API_KEY=your-key")
	}

	systemPrompt := quellSystemPrompt()
	response := callClaude(apiKey, systemPrompt, question)
	fmt.Println(response)
}

func cmdConvert() {
	if len(os.Args) < 3 {
		fatalf("usage: quell convert <file.py>")
	}
	src := readFile(os.Args[2])
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fatalf("ANTHROPIC_API_KEY not set — run: export ANTHROPIC_API_KEY=your-key")
	}

	prompt := fmt.Sprintf(`Convert the following Python quantum code to Quell language.

Quell syntax: one gate per line, uppercase gate name, then qubit indices, then args.
Examples:
  H 0         (Hadamard on qubit 0)
  CNOT 0 1    (CNOT, control=0, target=1)
  RX 1.5708 0 (RX rotation by pi/2 on qubit 0)
  MEASURE     (measure all)

Only output valid Quell code with comments explaining any non-obvious choices. No Python.

Python code:
%s`, src)

	response := callClaude(apiKey, "You are a quantum programming expert who converts Python/Qiskit code to Quell language.", prompt)
	fmt.Println(response)
}

func callClaude(apiKey, systemPrompt, userMessage string) string {
	body := map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 2048,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	}

	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("API error: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(data, &result)

	if result.Error.Message != "" {
		fatalf("Claude error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		fatalf("empty response from Claude")
	}
	return result.Content[0].Text
}

func quellSystemPrompt() string {
	return `You are the Quell AI assistant — a helpful expert on the Quell quantum circuit language and QubitLabs learning platform.

Quell is an open-source, backend-agnostic quantum programming language built by Magnobit.
- Website: https://qubitlabs.magnobit.com
- Repo: https://github.com/magnobit/quell
- Simple one-gate-per-line syntax
- Compiles to OpenQASM, Qiskit, Cirq, or Braket
- Companies bring their own backend credentials via quell.config.yml

You help users:
1. Write Quell circuits
2. Understand quantum algorithms
3. Convert between Quell and other quantum languages
4. Understand which backend to use for their use case

Keep answers concise and include code examples in Quell where relevant.`
}

func loadConfig() *config.Config {
	paths := []string{"quell.config.yml", "quell.config.yaml"}
	if len(os.Args) > 3 {
		for i, a := range os.Args {
			if a == "--config" && i+1 < len(os.Args) {
				paths = []string{os.Args[i+1]}
			}
		}
	}
	for _, p := range paths {
		cfg, err := config.Load(p)
		if err == nil {
			return cfg
		}
	}
	return config.Default()
}

func readFile(path string) string {
	if !strings.HasSuffix(path, ".quell") && !strings.HasSuffix(path, ".py") {
		fatalf("expected .quell or .py file, got: %s", filepath.Ext(path))
	}
	data, err := os.ReadFile(path)
	must(err, "cannot read file")
	return string(data)
}

func must(err error, msg string) {
	if err != nil {
		fatalf("%s: %v", msg, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "quell: "+format+"\n", args...)
	os.Exit(1)
}

func printHelp() {
	fmt.Print(`Quell — backend-agnostic quantum circuit language

Usage:
  quell run <file.quell>                Run circuit (local sim or configured backend)
  quell compile <file.quell>            Compile to OpenQASM (default)
    --target openqasm|qiskit|cirq|braket
    --output <file>
    --config <quell.config.yml>
  quell ask "<question>"                AI assistant (needs ANTHROPIC_API_KEY)
  quell convert <file.py>               Convert Python/Qiskit to Quell
  quell version                         Print version
  quell help                            Print this help

Examples:
  quell run examples/bell.quell
  quell compile --target qiskit bell.quell
  quell ask "how does Grover's algorithm work?"
  quell convert my_qiskit_circuit.py

Docs: https://github.com/magnobit/quell
Try:  https://qubitlabs.magnobit.com
`)
}
