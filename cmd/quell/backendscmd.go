// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/magnobit/quell/estimate"
	"github.com/magnobit/quell/internal/backends"
	"github.com/spf13/cobra"
)

func newBackendsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backends",
		Short: "List and inspect known backends, or check a circuit's compatibility",
		Long: `List and inspect the backends Quell knows about.

This is a local, offline catalog — it doesn't need a QubitLabs account or
network access, and doesn't reflect live queue/calibration data (see
QubitLabs Cloud's scheduler for that). It's the same backend set as
qubitlabs-platform's Control Plane catalog.`,
	}
	cmd.AddCommand(newBackendsListCmd())
	cmd.AddCommand(newBackendsInspectCmd())
	cmd.AddCommand(newBackendsCompatibleCmd())
	return cmd
}

func newBackendsListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List known backends",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := backends.Catalog()
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}
			printBackendsTable(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

func newBackendsInspectCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Print one backend's full catalog entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, ok := backends.Lookup(args[0])
			if !ok {
				return fmt.Errorf("unknown backend %q — run `quell backends list` to see known IDs", args[0])
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(entry)
			}
			fmt.Printf("%s (%s)\n", entry.Label, entry.ID)
			fmt.Printf("  Execution model : %s\n", entry.ExecutionModel)
			fmt.Printf("  Status          : %s\n", entry.Status)
			fmt.Printf("  Readiness       : %s\n", entry.Readiness.Level)
			if entry.Readiness.Evidence != "" {
				fmt.Printf("  Evidence        : %s\n", entry.Readiness.Evidence)
			}
			if entry.Readiness.Notes != "" {
				fmt.Printf("  Notes           : %s\n", entry.Readiness.Notes)
			}
			if entry.Qubits > 0 {
				fmt.Printf("  Qubits          : %d\n", entry.Qubits)
			} else {
				fmt.Printf("  Qubits          : ? (not publicly fixed, or unknown)\n")
			}
			fmt.Printf("  Description     : %s\n", entry.Description)
			param := "unknown"
			if entry.SupportsParameterizedCircuits != nil {
				if *entry.SupportsParameterizedCircuits {
					param = "yes (QubitLabs bind-then-submit; not native provider PARAM)"
				} else {
					param = "no"
				}
			}
			fmt.Printf("  Parameterized   : %s\n", param)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

func newBackendsCompatibleCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "compatible <file.quell>",
		Short: "Check which known backends a circuit's requirements are compatible with",
		Long: `Derives the circuit's requirements locally (qubits, execution model, and
required operations) and checks them against each catalog entry's known
capabilities. This is a static, offline check from the same provider
registry the server uses.

For parameterized, mid-circuit, or dynamic-control requirements, UNKNOWN
or false is incompatible — support must be confirmed. Parameterized
support is QubitLabs bind-then-submit, not native provider PARAM.
Unknown qubit counts are marked "?" and do not fail the verdict.`,
		Example: "  quell backends compatible chemistry.quell",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			req, err := estimate.DeriveRequirements(string(data), 0)
			if err != nil {
				return err
			}

			results := compatibilityResults(req)
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"requirements": req, "backends": results})
			}
			printCompatibilityTable(req, results)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

// backendCompatibility is one row of the `backends compatible` report.
type backendCompatibility struct {
	ID             string `json:"id"`
	ExecutionModel string `json:"executionModelCheck"` // PASS | FAIL
	Qubits         string `json:"qubitsCheck"`         // PASS | FAIL | "?"
	Parameterized  string `json:"parameterizedCheck"`  // PASS | FAIL | "?" | ""
	MidCircuit     string `json:"midCircuitCheck"`     // PASS | FAIL | "?" | ""
	Dynamic        string `json:"dynamicCheck"`        // PASS | FAIL | "?" | ""
	Compatible     bool   `json:"compatible"`
}

func compatibilityResults(req estimate.WorkloadRequirements) []backendCompatibility {
	entries := backends.Catalog()
	out := make([]backendCompatibility, 0, len(entries))
	for _, e := range entries {
		execOK := executionModelCompatible(req.ExecutionModel, e.ExecutionModel)
		execCheck := "FAIL"
		if execOK {
			execCheck = "PASS"
		}

		qubitsCheck := "?"
		qubitsOK := true // unknown qubit count never fails the overall verdict
		if e.Qubits > 0 {
			qubitsOK = e.Qubits >= req.LogicalQubits
			if qubitsOK {
				qubitsCheck = "PASS"
			} else {
				qubitsCheck = "FAIL"
			}
		}

		paramCheck := ""
		paramOK := true
		if req.UsesParameterizedGates {
			if e.SupportsParameterizedCircuits == nil {
				paramCheck = "?"
				paramOK = false // unknown is not supported
			} else if *e.SupportsParameterizedCircuits {
				paramCheck = "PASS"
			} else {
				paramCheck = "FAIL"
				paramOK = false
			}
		}

		// Offline catalog does not invent mid-circuit / dynamic support.
		// Required + UNKNOWN matches the server: incompatible.
		midCheck, midOK := optionalCapabilityCheck(req.MidCircuitMeasurement, nil)
		dynCheck, dynOK := optionalCapabilityCheck(req.DynamicControl, nil)

		out = append(out, backendCompatibility{
			ID:             e.ID,
			ExecutionModel: execCheck,
			Qubits:         qubitsCheck,
			Parameterized:  paramCheck,
			MidCircuit:     midCheck,
			Dynamic:        dynCheck,
			Compatible:     execOK && qubitsOK && paramOK && midOK && dynOK,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func optionalCapabilityCheck(required bool, known *bool) (check string, ok bool) {
	if !required {
		return "", true
	}
	if known == nil {
		return "?", false
	}
	if *known {
		return "PASS", true
	}
	return "FAIL", false
}

// executionModelCompatible mirrors qubitlabs-platform's scheduler rule
// (internal/scheduler/engine.go's executionModelCompatible): a gate-model
// workload — which is what every Quell circuit is — can run on gate or
// simulation backends, never annealing ones. Kept in sync by hand since
// this is a separate Go module from the platform.
func executionModelCompatible(required, backend estimate.ExecutionModel) bool {
	switch required {
	case estimate.ExecGate:
		return backend == estimate.ExecGate || backend == estimate.ExecSimulation
	case estimate.ExecAnnealing:
		return backend == estimate.ExecAnnealing
	case estimate.ExecSimulation:
		return backend == estimate.ExecSimulation
	default:
		return true // unknown requirement — don't manufacture a rejection
	}
}

func printBackendsTable(entries []backends.CatalogEntry) {
	fmt.Printf("%-12s %-10s %-8s %-8s %s\n", "BACKEND", "MODEL", "QUBITS", "STATUS", "READINESS")
	for _, e := range entries {
		qubits := "?"
		if e.Qubits > 0 {
			qubits = fmt.Sprintf("%d", e.Qubits)
		}
		fmt.Printf("%-12s %-10s %-8s %-8s %s\n", e.ID, e.ExecutionModel, qubits, e.Status, e.Readiness.Level)
	}
}

func printCompatibilityTable(req estimate.WorkloadRequirements, results []backendCompatibility) {
	fmt.Printf("Workload requirements\n")
	fmt.Printf("  Execution model : %s\n", req.ExecutionModel)
	fmt.Printf("  Logical qubits  : %d\n", req.LogicalQubits)
	fmt.Printf("  Required ops    : %v\n", req.RequiredOps)
	if req.MidCircuitMeasurement {
		fmt.Printf("  Mid-circuit measurement required\n")
	}
	if req.DynamicControl {
		fmt.Printf("  Dynamic control (IF/WHILE/SWITCH) required\n")
	}
	if req.UsesParameterizedGates {
		fmt.Printf("  Uses parameterized gates (PARAM)\n")
	}
	if len(req.RequiredConnectivity) > 0 {
		fmt.Printf("  Required connectivity: %v (native fit vs. a specific backend needs its live topology — see QubitLabs Cloud's scheduler)\n", req.RequiredConnectivity)
	}
	fmt.Println()
	fmt.Printf("%-12s %-8s %-8s %-8s %s\n", "BACKEND", "MODEL", "QUBITS", "PARAM", "COMPATIBLE")
	for _, r := range results {
		verdict := "no"
		if r.Compatible {
			verdict = "yes"
		}
		param := r.Parameterized
		if param == "" {
			param = "-"
		}
		fmt.Printf("%-12s %-8s %-8s %-8s %s\n", r.ID, r.ExecutionModel, r.Qubits, param, verdict)
	}
}
