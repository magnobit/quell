// Copyright 2026 Magnobit. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/parser"
)

const maxRequestBytes = 1 << 20 // 1 MB

func cmdServe() {
	port := os.Getenv("PORT")
	if port == "" {
		args := os.Args[2:]
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--port" {
				port = args[i+1]
				break
			}
		}
	}
	if port == "" {
		port = "8081"
	}

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
			Code   string `json:"code"`
			Target string `json:"target"`
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

		target := compiler.Target(req.Target)
		if target == "" {
			target = compiler.TargetOpenQASM
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

		result, err := compiler.Compile(circ, target)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{
				"error":     err.Error(),
				"errorType": "compile",
			})
			return
		}

		lang := "python"
		if target == compiler.TargetOpenQASM {
			lang = "openqasm"
		}

		json.NewEncoder(w).Encode(map[string]string{
			"result":   result,
			"target":   string(target),
			"language": lang,
		})
	})

	fmt.Printf("Quell compile server v%s listening on :%s\n", version, port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fatalf("server: %v", err)
	}
}
