// Copyright 2026 Magnobit, Inc. All rights reserved.

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

	"github.com/magnobit/quell/convert"
	"github.com/magnobit/quell/internal/qasmimport"
	"github.com/spf13/cobra"
)

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ask <question>",
		Short:   "AI assistant for Quell and quantum computing (needs ANTHROPIC_API_KEY)",
		Example: `  quell ask "how does Grover's algorithm work?"`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey := os.Getenv("ANTHROPIC_API_KEY")
			if apiKey == "" {
				return fmt.Errorf("ANTHROPIC_API_KEY not set — run: export ANTHROPIC_API_KEY=your-key")
			}
			question := strings.Join(args, " ")
			response, err := callClaude(apiKey, quellSystemPrompt(), question)
			if err != nil {
				return err
			}
			fmt.Println(response)
			return nil
		},
	}
}

func newConvertCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "convert <file.py|file.qasm|file.qs>",
		Short: "Convert OpenQASM (local) or Python/Q# (AI) to Quell",
		Long: `Convert circuits into Quell.

  • .qasm / .qasm2 / .qasm3 — local OpenQASM → Quell (no API key; # // /* */ comments ignored)
  • .py / .qs — Claude-assisted when ANTHROPIC_API_KEY is set
  • For deterministic Qiskit/Cirq/Q#/Braket import without an API key, use Labs Migrate
    or POST /api/v1/ai/convert (language=qiskit|cirq|qsharp|braket|openqasm).

Export Quell → other languages with:
  quell compile --target qiskit|cirq|openqasm|openqasm2|braket|qsharp file.quell`,
		Example: `  quell convert bell.qasm
  quell convert my_qiskit_circuit.py
  quell compile --target qsharp bell.quell`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			lower := strings.ToLower(path)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			lang := convert.DetectLanguage(src, path)

			if strings.HasSuffix(lower, ".qasm") || strings.HasSuffix(lower, ".qasm2") || strings.HasSuffix(lower, ".qasm3") || lang == "openqasm" {
				out, err := qasmimport.ToQuell(src)
				if err != nil {
					return fmt.Errorf("qasm import: %w", err)
				}
				fmt.Print(out)
				return nil
			}

			apiKey := os.Getenv("ANTHROPIC_API_KEY")
			if apiKey == "" {
				return fmt.Errorf("%s → Quell needs ANTHROPIC_API_KEY for CLI, or use Labs Migrate / POST /api/v1/ai/convert with language=%s (OpenQASM files convert locally without a key)", lang, lang)
			}
			ext := filepath.Ext(path)
			if ext != ".py" && ext != ".qs" && ext != ".qsharp" && ext != ".quell" {
				return fmt.Errorf("expected .py, .qs, .qasm, or .qasm3 file, got: %s", ext)
			}
			prompt := fmt.Sprintf(`Convert the following %s quantum code to Quell language.

Quell syntax: one gate per line, uppercase gate name, then qubit indices, then args.
Examples:
  H 0
  CNOT 0 1
  RX 1.5708 0
  MEASURE

Only output valid Quell code. Ignore commented-out gates.

Source (%s):
%s`, lang, lang, src)

			response, err := callClaude(apiKey, "You are a quantum programming expert who converts Qiskit, Cirq, Q#, Braket, and OpenQASM to Quell.", prompt)
			if err != nil {
				return err
			}
			fmt.Println(response)
			return nil
		},
	}
}

func callClaude(apiKey, systemPrompt, userMessage string) (string, error) {
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
		return "", fmt.Errorf("API error: %w", err)
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
		return "", fmt.Errorf("Claude error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return result.Content[0].Text, nil
}

func quellSystemPrompt() string {
	return `You are the Quell AI assistant — a helpful expert on the Quell quantum circuit language and QubitLabs learning platform.

Quell is an open-source, backend-agnostic quantum programming language built by Magnobit.
- Website: https://qubitlabs.magnobit.com
- Repo: https://github.com/magnobit/quell
- Simple one-gate-per-line syntax
- Compiles to OpenQASM, Qiskit, Cirq, Braket, or Q#
- Companies bring their own backend credentials via quell.config.yml or CLI flags

You help users:
1. Write Quell circuits
2. Understand quantum algorithms
3. Convert between Quell and other quantum languages
4. Understand which backend to use for their use case

Keep answers concise and include code examples in Quell where relevant.`
}
