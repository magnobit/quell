// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/magnobit/quell/compile"
	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/spf13/cobra"
)

const maxRequestBytes = 1 << 20 // 1 MB

func newServeCmd() *cobra.Command {
	var port string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP compile server",
		Example: `  quell serve
  quell serve --port 9000`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("port") {
				if p := os.Getenv("PORT"); p != "" {
					port = p
				}
			}
			return serve(port)
		},
	}

	cmd.Flags().StringVar(&port, "port", "8081", "port to listen on (env PORT)")
	return cmd
}

func serve(port string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "quell-compiler",
			"version": version,
		})
	})

	mux.HandleFunc("OPTIONS /compile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /compile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		// Panic recovery — a compiler bug must not crash the server
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("compile panic: %v\n%s", rec, debug.Stack())
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("internal compiler error: %v", rec),
				})
			}
		}()

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

		var req struct {
			Code     string   `json:"code"`
			Target   string   `json:"target"`
			Targets  []string `json:"targets"`
			Optimize *bool    `json:"optimize"` // defaults to true when omitted
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			status := http.StatusBadRequest
			msg := "invalid request body"
			if err.Error() == "http: request body too large" {
				status = http.StatusRequestEntityTooLarge
				msg = fmt.Sprintf("request body exceeds %d bytes", maxRequestBytes)
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"error": msg})
			return
		}
		if req.Code == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "code is required"})
			return
		}

		spec := strings.TrimSpace(req.Target)
		if len(req.Targets) > 0 {
			spec = strings.Join(req.Targets, ",")
		}
		if spec == "" {
			spec = "openqasm"
		}
		targets, terr := compile.ResolveTargets(spec)
		if terr != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": terr.Error(), "errorType": "validation"})
			return
		}

		circ, err := parser.Parse(req.Code)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{
				"error":     err.Error(),
				"errorType": "parse",
			})
			return
		}
		if err := check.Fail(circ); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{
				"error":     err.Error(),
				"errorType": "check",
			})
			return
		}

		optimize := true
		if req.Optimize != nil {
			optimize = *req.Optimize
		}

		emitted, err := compile.Emit(ir.Lower(circ), targets, optimize)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{
				"error":     err.Error(),
				"errorType": "compile",
			})
			return
		}

		if len(targets) == 1 {
			target := targets[0]
			one := emitted[target]
			lang := "python"
			if target == compile.OpenQASM || target == compile.OpenQASM2 {
				lang = "openqasm"
			}
			if target == compile.QSharp {
				lang = "qsharp"
			}
			json.NewEncoder(w).Encode(map[string]any{
				"result":         one.Code,
				"target":         string(target),
				"language":       lang,
				"optimizerNotes": one.OptimizerNotes,
				"fingerprint":    compile.Fingerprint(req.Code, string(target), optimize),
			})
			return
		}

		results := map[string]string{}
		prints := map[string]string{}
		var notes []string
		for _, target := range targets {
			one := emitted[target]
			results[string(target)] = one.Code
			prints[string(target)] = compile.Fingerprint(req.Code, string(target), optimize)
			notes = one.OptimizerNotes
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results":        results,
			"fingerprints":   prints,
			"optimizerNotes": notes,
		})
	})

	fmt.Printf("Quell compile server v%s listening on :%s\n", version, port)
	return http.ListenAndServe(":"+port, mux)
}
