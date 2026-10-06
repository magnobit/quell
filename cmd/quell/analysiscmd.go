// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/magnobit/quell/analysis"
	"github.com/spf13/cobra"
)

func loadAnalysis(path string) (*analysis.Model, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return analysis.Load(string(src))
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// pickObservable returns the requested observable, or the only declared one.
func pickObservable(m *analysis.Model, name string) (string, error) {
	names := m.Observables()
	if name != "" {
		return name, nil
	}
	switch len(names) {
	case 0:
		return "", fmt.Errorf("circuit declares no observable; add `observable h = Z(0)` or pass --observable")
	case 1:
		return names[0], nil
	}
	return "", fmt.Errorf("circuit declares %d observables %v; choose one with --observable", len(names), names)
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newDrawCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "draw <file.quell>",
		Short:   "Draw the circuit as ASCII, one row per qubit",
		Example: "  quell draw bell.quell",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadAnalysis(args[0])
			if err != nil {
				return err
			}
			fmt.Print(analysis.Draw(m.Program()))
			return nil
		},
	}
}

func newStateCmd() *cobra.Command {
	var paramFlags []string
	var top int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "state <file.quell>",
		Short: "Print the exact final statevector of the unitary prefix (local, ideal, no noise)",
		Long: `Evolves the gates before the first MEASURE on the local statevector and
prints the largest amplitudes. Bit strings are MSB-first with qubit 0 on the
right, the same as simulation counts. Circuits with RESET, mid-circuit
measurement, or classical control are rejected: they have no single pure state.`,
		Example: "  quell state bell.quell\n  quell state ansatz.quell --param theta=0.7 --top 8 --json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadAnalysis(args[0])
			if err != nil {
				return err
			}
			params, err := analysis.ParseParams(paramFlags)
			if err != nil {
				return err
			}
			sv, err := m.State(params)
			if err != nil {
				return err
			}
			amps := analysis.TopAmplitudes(sv, top, 1e-12)
			if asJSON {
				return printJSON(map[string]any{
					"backend": analysis.Backend, "qubits": sv.N, "amplitudes": amps,
					"prefixOnly": m.PrefixOnly(),
				})
			}
			fmt.Printf("Backend : %s (ideal, no noise)\nQubits  : %d\n", analysis.Backend, sv.N)
			if m.PrefixOnly() {
				fmt.Println("Note    : gates after the first MEASURE are not included; this is the state at that measurement.")
			}
			for _, a := range amps {
				fmt.Printf("|%s>  %+.6f%+.6fi  p=%.6f\n", a.Bits, a.Re, a.Im, a.Prob)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "bind symbolic angle: --param theta=1.5708 (repeatable)")
	cmd.Flags().IntVar(&top, "top", 16, "show at most this many basis states (0 = all non-zero)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

func newObserveCmd() *cobra.Command {
	var paramFlags []string
	var observable string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "observe <file.quell>",
		Short: "Exact expectation value of an observable on the local statevector",
		Long: `Evaluates <psi|O|psi> exactly for an observable declared with
` + "`observable h = -1.05 * Z(0) + 0.18 * X(0) * X(1)`" + `. This is the exact
statevector expectation, not a sampled estimate, and not a provider-native
result.`,
		Example: "  quell observe h2.quell\n  quell observe ansatz.quell --observable h --param theta=0.7",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadAnalysis(args[0])
			if err != nil {
				return err
			}
			params, err := analysis.ParseParams(paramFlags)
			if err != nil {
				return err
			}
			name, err := pickObservable(m, observable)
			if err != nil {
				return err
			}
			v, err := m.Expectation(name, params)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(map[string]any{
					"observable": name, "value": v, "params": params,
					"backend": analysis.Backend, "strategy": "statevector", "shots": nil,
					"prefixOnly": m.PrefixOnly(),
				})
			}
			fmt.Printf("Observable : %s\nExpectation: %.10f\nBackend    : %s (exact statevector, no sampling)\n", name, v, analysis.Backend)
			if m.PrefixOnly() {
				fmt.Println("Note       : gates after the first MEASURE are not included.")
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "bind symbolic angle: --param theta=1.5708 (repeatable)")
	cmd.Flags().StringVar(&observable, "observable", "", "observable name (default: the only one declared)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

func newGradientCmd() *cobra.Command {
	var paramFlags, wrt []string
	var observable string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "gradient <file.quell>",
		Short: "Gradient of an observable expectation with respect to PARAMs (central difference)",
		Long: `Computes d<O>/d(param) at the given point with a central finite
difference on the exact local expectation. It is not the parameter-shift rule.
Every PARAM must be bound with --param; --wrt limits which derivatives print.`,
		Example: "  quell gradient ansatz.quell --param theta=0.7\n  quell gradient qaoa.quell --observable cost --param gamma=0.3 --param beta=0.2 --wrt gamma",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadAnalysis(args[0])
			if err != nil {
				return err
			}
			params, err := analysis.ParseParams(paramFlags)
			if err != nil {
				return err
			}
			name, err := pickObservable(m, observable)
			if err != nil {
				return err
			}
			g, err := m.Gradient(name, params, wrt)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(map[string]any{
					"observable": name, "gradient": g, "params": params,
					"backend": analysis.Backend, "strategy": "central-difference",
				})
			}
			fmt.Printf("Observable: %s\nStrategy  : central difference on exact statevector expectation\n", name)
			for _, k := range sortedKeys(g) {
				fmt.Printf("d/d%s = %+.8f\n", k, g[k])
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "bind symbolic angle: --param theta=1.5708 (repeatable)")
	cmd.Flags().StringSliceVar(&wrt, "wrt", nil, "parameters to differentiate (default: all)")
	cmd.Flags().StringVar(&observable, "observable", "", "observable name (default: the only one declared)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}

func newVQECmd() *cobra.Command {
	var paramFlags []string
	var observable string
	var maxIter int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "vqe <file.quell>",
		Short: "Minimise an observable over the circuit's PARAMs (local, exact, Nelder-Mead)",
		Long: `Runs a classical Nelder-Mead loop around the exact local expectation
to minimise <O> over every PARAM. --param sets the starting point; any PARAM
not given starts at 0.1 radians. The result is a local-statevector
optimisation, not a provider run. It may reach a local minimum.`,
		Example: "  quell vqe h2_ansatz.quell\n  quell vqe ansatz.quell --observable h --param theta=0.5 --max-iter 800",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadAnalysis(args[0])
			if err != nil {
				return err
			}
			start, err := analysis.ParseParams(paramFlags)
			if err != nil {
				return err
			}
			name, err := pickObservable(m, observable)
			if err != nil {
				return err
			}
			res, err := m.Minimize(name, start, analysis.MinOptions{MaxIter: maxIter})
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(map[string]any{"observable": name, "result": res})
			}
			fmt.Printf("Observable : %s\nMinimum    : %.10f\nConverged  : %v (%d iterations, %d evaluations)\nStrategy   : %s on %s\n",
				name, res.Value, res.Converged, res.Iterations, res.Evaluations, res.Strategy, res.Backend)
			for _, k := range sortedKeys(res.Params) {
				fmt.Printf("%s = %.8f\n", k, res.Params[k])
			}
			if !res.Converged {
				fmt.Println("Note: iteration cap reached; value is the best point seen, not a proven minimum.")
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "starting value: --param theta=0.5 (repeatable)")
	cmd.Flags().StringVar(&observable, "observable", "", "observable name (default: the only one declared)")
	cmd.Flags().IntVar(&maxIter, "max-iter", 400, "Nelder-Mead iteration cap")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}
