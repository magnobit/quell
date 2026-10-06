// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/magnobit/quell/internal/cloudclient"
	"github.com/spf13/cobra"
)

func newCloudCmd() *cobra.Command {
	var endpoint, apiKey, configPath string
	cloud := &cobra.Command{
		Use:   "cloud",
		Short: "Call the QubitLabs public job API",
		Long: `Submit and read jobs through the QubitLabs job API.

The API key is QUBITLABS_API_KEY or --api-key. The endpoint is
QUBITLABS_ENDPOINT, cloud.endpoint in quell.config.yml, or --endpoint.
This path uses the same scheduler as the console. It does not call a provider with a hardware token.`,
	}
	cloud.PersistentFlags().StringVar(&endpoint, "endpoint", "", "QubitLabs API origin (env QUBITLABS_ENDPOINT)")
	cloud.PersistentFlags().StringVar(&apiKey, "api-key", "", "QubitLabs API key (env QUBITLABS_API_KEY)")
	cloud.PersistentFlags().StringVar(&configPath, "config", "", "quell config file")

	client := func() *cloudclient.Client {
		cfg := loadConfigFrom(configPath)
		ep := firstNonEmpty(endpoint, os.Getenv("QUBITLABS_ENDPOINT"), cfg.Cloud.Endpoint)
		key := firstNonEmpty(apiKey, os.Getenv("QUBITLABS_API_KEY"))
		return &cloudclient.Client{Endpoint: ep, APIKey: key}
	}
	printJSON := func(v any) {
		raw, err := json.MarshalIndent(v, "", "  ")
		must(err, "encode")
		fmt.Println(string(raw))
	}

	cloud.AddCommand(&cobra.Command{
		Use:   "backends",
		Short: "List backends visible to this API key",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			backs, err := client().ListBackends()
			must(err, "cloud backends")
			printJSON(backs)
		},
	})

	var language, backend, idem string
	var shots int
	submit := &cobra.Command{
		Use:   "submit <file>",
		Short: "Submit a Quell, OpenQASM 3, or QIR file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			src, lang := readWorkload(args[0], language)
			job, err := client().Submit(src, lang, backend, shots, idem)
			must(err, "cloud submit")
			printJSON(job)
		},
	}
	submit.Flags().StringVar(&language, "language", "", "quell, openqasm3, or qir (default: from the file extension)")
	submit.Flags().StringVar(&backend, "backend", "auto", "backend id or auto")
	submit.Flags().IntVar(&shots, "shots", 1024, "shot count")
	submit.Flags().StringVar(&idem, "idempotency-key", "", "retry key; the same key with a different file is rejected")
	cloud.AddCommand(submit)

	cloud.AddCommand(jobCmd("status", "Show job status", func(c *cloudclient.Client, id string) (any, error) {
		return c.Get(id)
	}, client, printJSON))
	cloud.AddCommand(jobCmd("result", "Show the job result", func(c *cloudclient.Client, id string) (any, error) {
		return c.Result(id)
	}, client, printJSON))
	cloud.AddCommand(jobCmd("provenance", "Show job provenance", func(c *cloudclient.Client, id string) (any, error) {
		return c.Provenance(id)
	}, client, printJSON))
	cloud.AddCommand(jobCmd("verify", "Show Verify reports for the job", func(c *cloudclient.Client, id string) (any, error) {
		return c.Verify(id)
	}, client, printJSON))
	cloud.AddCommand(&cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a queued or running job",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			must(client().Cancel(args[0]), "cloud cancel")
			fmt.Println("cancelled", args[0])
		},
	})
	return cloud
}

func jobCmd(use, short string, fn func(*cloudclient.Client, string) (any, error), client func() *cloudclient.Client, printJSON func(any)) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <job-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			out, err := fn(client(), args[0])
			must(err, "cloud "+use)
			printJSON(out)
		},
	}
}

func readWorkload(path, language string) (string, string) {
	data, err := os.ReadFile(path)
	must(err, "cannot read file")
	if language == "" {
		switch {
		case strings.HasSuffix(path, ".quell"):
			language = "quell"
		case strings.HasSuffix(path, ".qasm") || strings.HasSuffix(path, ".qasm3"):
			language = "openqasm3"
		case strings.HasSuffix(path, ".ll") || strings.HasSuffix(path, ".qir"):
			language = "qir"
		default:
			fatalf("pass --language quell, openqasm3, or qir")
		}
	}
	return string(data), language
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
