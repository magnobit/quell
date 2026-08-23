// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	quelldocs "github.com/magnobit/quell"
	"github.com/magnobit/quell/askdocs"
	"github.com/magnobit/quell/convert"
	"github.com/magnobit/quell/migrate"
	"github.com/spf13/cobra"
)

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ask <question>",
		Short:   "AI assistant for Quell and quantum computing (Claude when ANTHROPIC_API_KEY is set, local doc search otherwise)",
		Example: `  quell ask "how does Grover's algorithm work?"`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			question := strings.Join(args, " ")
			apiKey := os.Getenv("ANTHROPIC_API_KEY")
			if apiKey == "" {
				fmt.Println(askdocs.Answer(question, quelldocs.Docs()))
				return nil
			}
			response, err := callClaude(apiKey, quellSystemPrompt(), question)
			if err != nil {
				fmt.Println(askdocs.Answer(question, quelldocs.Docs()))
				return nil
			}
			fmt.Println(response)
			return nil
		},
	}
}

func newConvertCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "convert <file.py|file.qasm|file.qs>",
		Short: "Convert OpenQASM/Qiskit/Cirq/Q#/Braket to Quell",
		Long: `Convert circuits into Quell.

  • .qasm / .qasm2 / .qasm3 — OpenQASM → Quell
  • .py — Qiskit/Cirq/Braket → Quell
  • .qs — Q# → Quell

All of the above convert locally — no API key, no network call. When the
local converter can't fully handle a Python file, set ANTHROPIC_API_KEY for
an LLM-assisted second pass (export ANTHROPIC_API_KEY=your-key).

Export Quell → other languages with:
  quell compile --target qiskit|cirq|openqasm|openqasm2|braket|qsharp file.quell`,
		Example: `  quell convert bell.qasm
  quell convert my_qiskit_circuit.py
  quell compile --target qsharp bell.quell`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			lang := convert.DetectLanguage(src, path)

			result, mErr := migrate.ToQuell(src, lang)
			if mErr == nil && strings.TrimSpace(result.Quell) != "" {
				fmt.Print(result.Quell)
				for _, w := range result.Warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", w)
				}
				return nil
			}

			apiKey := os.Getenv("ANTHROPIC_API_KEY")
			if apiKey == "" || !migrate.IsPythonish(lang, result.Language) {
				if mErr != nil {
					return mErr
				}
				return fmt.Errorf("conversion produced no Quell output")
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
