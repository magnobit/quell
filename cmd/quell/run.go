// Copyright 2026 Magnobit, Inc. All rights reserved.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magnobit/quell/internal/backends"
	"github.com/magnobit/quell/internal/compiler"
	"github.com/magnobit/quell/internal/config"
	"github.com/magnobit/quell/internal/ir"
	"github.com/magnobit/quell/internal/parser"
	"github.com/magnobit/quell/simulate"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var configPath, backendOverride string
	var setFlags []string

	cmd := &cobra.Command{
		Use:   "run <file.quell>",
		Short: "Run a circuit (local sim or a configured backend)",
		Long: `Run a circuit (local sim or a configured backend)

Credentials and per-backend parameters can come from quell.config.yml, from
environment variables, or straight from the command line — in that order of
precedence (a flag always wins). A parameter without a dedicated flag yet
can still be sent via --set <backend>.<key>=<value>; see 'quell run --help'
for the full flag list.`,
		Example: `  quell run bell.quell
  quell run bell.quell --backend ibm --ibm-token $IBM_TOKEN --ibm-device ibm_brisbane
  quell run bell.quell --backend azure --azure-tenant-id $TID --azure-client-id $CID \
    --azure-client-secret $SECRET --azure-subscription-id $SUB \
    --azure-resource-group $RG --azure-workspace $WS --azure-target ionq.simulator
  quell run bell.quell --backend ionq --ionq-api-key $KEY --set ionq.error_mitigation=true`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.HasSuffix(args[0], ".quell") {
				return fmt.Errorf("expected a .quell file, got: %s", args[0])
			}
			// ParseFile (not Parse) so "import" lines resolve relative to
			// this file's directory, or against an installed package.
			circ, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			cfg := loadConfigFrom(configPath)
			applyRunFlags(cmd, cfg, backendOverride)
			if err := applySetFlags(cfg, setFlags); err != nil {
				return err
			}

			fmt.Printf("Backend : %s\n", cfg.Backend)
			fmt.Printf("Qubits  : %d\n", circ.NumQubits)
			fmt.Printf("Gates   : %d\n\n", len(circ.Instructions))

			return runOnBackend(cfg, circ)
		},
	}

	f := cmd.Flags()
	f.StringVar(&configPath, "config", "", "path to quell.config.yml (default: ./quell.config.yml or .yaml)")
	f.StringVar(&backendOverride, "backend", "", "backend: local|ibm|aws|google|ionq|rigetti|azure|dwave (overrides config file)")
	f.StringArrayVar(&setFlags, "set", nil, "generic backend param not covered by a typed flag below: --set <backend>.<key>=<value> (repeatable)")

	f.Int("shots", 0, "shots for the local backend")

	f.String("ibm-token", "", "IBM Quantum API token (env IBM_QUANTUM_TOKEN)")
	f.String("ibm-instance", "", "IBM instance, e.g. hub/group/project (default ibm-q/open/main)")
	f.String("ibm-device", "", "IBM device, e.g. ibm_brisbane")
	f.Int("ibm-shots", 0, "shots for IBM Quantum")

	f.String("aws-access-key-id", "", "AWS access key ID (env AWS_ACCESS_KEY_ID — preferred, matches the AWS CLI convention)")
	f.String("aws-secret-access-key", "", "AWS secret access key (env AWS_SECRET_ACCESS_KEY — preferred)")
	f.String("aws-session-token", "", "AWS session token, for temporary credentials (env AWS_SESSION_TOKEN)")
	f.String("aws-region", "", "AWS region (default us-east-1)")
	f.String("aws-device", "", "Braket device ARN")
	f.String("aws-s3-bucket", "", "S3 bucket for Braket results")
	f.String("aws-s3-prefix", "", "S3 key prefix for Braket results (default quell-results)")
	f.Int("aws-shots", 0, "shots for AWS Braket")

	f.String("google-project", "", "GCP project ID")
	f.String("google-processor", "", "Google Quantum processor, e.g. rainbow, weber")
	f.String("google-key-file", "", "path to Google service account JSON key file")
	f.Int("google-shots", 0, "shots for Google Quantum Engine")

	f.String("rigetti-api-key", "", "Rigetti QCS API key (env RIGETTI_API_KEY)")
	f.String("rigetti-device", "", "Rigetti device, e.g. Aspen-M-3")
	f.Int("rigetti-shots", 0, "shots for Rigetti QCS")

	f.String("ionq-api-key", "", "IonQ API key (env IONQ_API_KEY)")
	f.String("ionq-device", "", "IonQ device, e.g. simulator, qpu.harmony")
	f.Int("ionq-shots", 0, "shots for IonQ Cloud")

	f.String("azure-tenant-id", "", "Azure AD tenant ID (env AZURE_TENANT_ID)")
	f.String("azure-client-id", "", "Azure AD app client ID (env AZURE_CLIENT_ID)")
	f.String("azure-client-secret", "", "Azure AD app client secret (env AZURE_CLIENT_SECRET)")
	f.String("azure-subscription-id", "", "Azure subscription ID (env AZURE_SUBSCRIPTION_ID)")
	f.String("azure-resource-group", "", "Azure resource group")
	f.String("azure-workspace", "", "Azure Quantum workspace name")
	f.String("azure-target", "", "Azure Quantum target, e.g. ionq.simulator")
	f.Int("azure-shots", 0, "shots for Azure Quantum")

	f.String("dwave-api-token", "", "D-Wave API token (env DWAVE_API_TOKEN)")
	f.String("dwave-solver", "", "D-Wave solver name")
	f.Int("dwave-shots", 0, "shots for D-Wave")

	return cmd
}

// applyRunFlags layers CLI-flag values (and, for the fields listed, their
// environment-variable fallback) on top of cfg as loaded from the config
// file. Precedence, low to high: config file → env var → explicit flag.
func applyRunFlags(cmd *cobra.Command, cfg *config.Config, backendOverride string) {
	if cmd.Flags().Changed("backend") {
		cfg.Backend = backendOverride
	}

	cfg.Local.Shots = resolveInt(cmd, "shots", cfg.Local.Shots)

	cfg.IBM.Token = resolveStr(cmd, "ibm-token", "IBM_QUANTUM_TOKEN", cfg.IBM.Token)
	cfg.IBM.Instance = resolveStr(cmd, "ibm-instance", "", cfg.IBM.Instance)
	cfg.IBM.Device = resolveStr(cmd, "ibm-device", "", cfg.IBM.Device)
	cfg.IBM.Shots = resolveInt(cmd, "ibm-shots", cfg.IBM.Shots)

	cfg.AWS.AccessKeyID = resolveStr(cmd, "aws-access-key-id", "AWS_ACCESS_KEY_ID", cfg.AWS.AccessKeyID)
	cfg.AWS.SecretAccessKey = resolveStr(cmd, "aws-secret-access-key", "AWS_SECRET_ACCESS_KEY", cfg.AWS.SecretAccessKey)
	cfg.AWS.SessionToken = resolveStr(cmd, "aws-session-token", "AWS_SESSION_TOKEN", cfg.AWS.SessionToken)
	cfg.AWS.Region = resolveStr(cmd, "aws-region", "AWS_REGION", cfg.AWS.Region)
	cfg.AWS.Device = resolveStr(cmd, "aws-device", "", cfg.AWS.Device)
	cfg.AWS.S3Bucket = resolveStr(cmd, "aws-s3-bucket", "", cfg.AWS.S3Bucket)
	cfg.AWS.S3Prefix = resolveStr(cmd, "aws-s3-prefix", "", cfg.AWS.S3Prefix)
	cfg.AWS.Shots = resolveInt(cmd, "aws-shots", cfg.AWS.Shots)

	cfg.Google.Project = resolveStr(cmd, "google-project", "GOOGLE_CLOUD_PROJECT", cfg.Google.Project)
	cfg.Google.Processor = resolveStr(cmd, "google-processor", "", cfg.Google.Processor)
	cfg.Google.KeyFile = resolveStr(cmd, "google-key-file", "GOOGLE_APPLICATION_CREDENTIALS", cfg.Google.KeyFile)
	cfg.Google.Shots = resolveInt(cmd, "google-shots", cfg.Google.Shots)

	cfg.Rigetti.APIKey = resolveStr(cmd, "rigetti-api-key", "RIGETTI_API_KEY", cfg.Rigetti.APIKey)
	cfg.Rigetti.Device = resolveStr(cmd, "rigetti-device", "", cfg.Rigetti.Device)
	cfg.Rigetti.Shots = resolveInt(cmd, "rigetti-shots", cfg.Rigetti.Shots)

	cfg.IonQ.APIKey = resolveStr(cmd, "ionq-api-key", "IONQ_API_KEY", cfg.IonQ.APIKey)
	cfg.IonQ.Device = resolveStr(cmd, "ionq-device", "", cfg.IonQ.Device)
	cfg.IonQ.Shots = resolveInt(cmd, "ionq-shots", cfg.IonQ.Shots)

	cfg.Azure.TenantID = resolveStr(cmd, "azure-tenant-id", "AZURE_TENANT_ID", cfg.Azure.TenantID)
	cfg.Azure.ClientID = resolveStr(cmd, "azure-client-id", "AZURE_CLIENT_ID", cfg.Azure.ClientID)
	cfg.Azure.ClientSecret = resolveStr(cmd, "azure-client-secret", "AZURE_CLIENT_SECRET", cfg.Azure.ClientSecret)
	cfg.Azure.SubscriptionID = resolveStr(cmd, "azure-subscription-id", "AZURE_SUBSCRIPTION_ID", cfg.Azure.SubscriptionID)
	cfg.Azure.ResourceGroup = resolveStr(cmd, "azure-resource-group", "", cfg.Azure.ResourceGroup)
	cfg.Azure.Workspace = resolveStr(cmd, "azure-workspace", "", cfg.Azure.Workspace)
	cfg.Azure.Target = resolveStr(cmd, "azure-target", "", cfg.Azure.Target)
	cfg.Azure.Shots = resolveInt(cmd, "azure-shots", cfg.Azure.Shots)

	cfg.DWave.APIToken = resolveStr(cmd, "dwave-api-token", "DWAVE_API_TOKEN", cfg.DWave.APIToken)
	cfg.DWave.Solver = resolveStr(cmd, "dwave-solver", "", cfg.DWave.Solver)
	cfg.DWave.Shots = resolveInt(cmd, "dwave-shots", cfg.DWave.Shots)
}

// resolveStr returns, in ascending precedence: cur (already loaded from the
// config file), then envName's value if cur is still empty, then the flag's
// value if the user explicitly passed it on this invocation.
func resolveStr(cmd *cobra.Command, flagName, envName, cur string) string {
	v := cur
	if v == "" && envName != "" {
		if e := os.Getenv(envName); e != "" {
			v = e
		}
	}
	if cmd.Flags().Changed(flagName) {
		v, _ = cmd.Flags().GetString(flagName)
	}
	return v
}

func resolveInt(cmd *cobra.Command, flagName string, cur int) int {
	if cmd.Flags().Changed(flagName) {
		v, _ := cmd.Flags().GetInt(flagName)
		return v
	}
	return cur
}

// applySetFlags parses --set <backend>.<key>=<value> entries into the named
// backend's config.Extra map — the forward-compatible escape hatch for a
// provider parameter that doesn't have a typed flag yet (see
// internal/backends/extra.go for where it's merged into the request).
func applySetFlags(cfg *config.Config, sets []string) error {
	for _, s := range sets {
		dot := strings.Index(s, ".")
		eq := strings.Index(s, "=")
		if dot < 0 || eq < 0 || eq < dot {
			return fmt.Errorf("invalid --set %q — expected <backend>.<key>=<value>, e.g. --set azure.foo=bar", s)
		}
		backendName, key, value := s[:dot], s[dot+1:eq], s[eq+1:]
		extra, err := cfg.ExtraFor(backendName)
		if err != nil {
			return err
		}
		extra[key] = value
	}
	return nil
}

func runOnBackend(cfg *config.Config, circ *parser.Circuit) error {
	switch cfg.Backend {
	case "local", "":
		shots := cfg.Local.Shots
		if shots == 0 {
			shots = 1000
		}
		result, err := simulate.RunProgram(ir.Lower(circ), shots)
		if err != nil {
			return fmt.Errorf("simulate error: %w", err)
		}
		result.Print()
		return nil

	case "ibm":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to IBM Quantum…")
		result, err := backends.RunIBM(&cfg.IBM, qasm3, circ.NumQubits)
		if err != nil {
			return fmt.Errorf("IBM run error: %w", err)
		}
		result.Print()
		return nil

	case "aws":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to AWS Braket…")
		result, err := backends.RunBraket(&cfg.AWS, qasm3)
		if err != nil {
			return fmt.Errorf("Braket run error: %w", err)
		}
		result.Print()
		return nil

	case "google":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to Google Quantum Engine…")
		result, err := backends.RunGoogle(&cfg.Google, qasm3)
		if err != nil {
			return fmt.Errorf("Google run error: %w", err)
		}
		result.Print()
		return nil

	case "ionq":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to IonQ Cloud…")
		result, err := backends.RunIonQ(&cfg.IonQ, qasm3, circ.NumQubits)
		if err != nil {
			return fmt.Errorf("IonQ run error: %w", err)
		}
		result.Print()
		return nil

	case "rigetti":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to Rigetti QCS…")
		result, err := backends.RunRigetti(&cfg.Rigetti, qasm3)
		if err != nil {
			return fmt.Errorf("Rigetti run error: %w", err)
		}
		result.Print()
		return nil

	case "azure":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		fmt.Println("  Compiled to OpenQASM 3, submitting to Azure Quantum…")
		result, err := backends.RunAzure(&cfg.Azure, qasm3)
		if err != nil {
			return fmt.Errorf("Azure run error: %w", err)
		}
		result.Print()
		return nil

	case "dwave":
		qasm3, _, err := compiler.Compile(circ, compiler.TargetOpenQASM, true)
		if err != nil {
			return fmt.Errorf("compile error: %w", err)
		}
		if _, err := backends.RunDWave(&cfg.DWave, qasm3); err != nil {
			return fmt.Errorf("D-Wave run error: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown backend %q — valid options: local, ibm, aws, google, ionq, rigetti, azure, dwave", cfg.Backend)
	}
}
