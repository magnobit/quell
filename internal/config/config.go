// Copyright 2026 Magnobit, Inc. All rights reserved.

package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Backend string        `yaml:"backend"`
	Local   LocalConfig   `yaml:"local"`
	IBM     IBMConfig     `yaml:"ibm"`
	AWS     AWSConfig     `yaml:"aws"`
	Google  GCPConfig     `yaml:"google"`
	Rigetti RigettiConfig `yaml:"rigetti"`
	IonQ    IonQConfig    `yaml:"ionq"`
	Azure   AzureConfig   `yaml:"azure"`
	DWave   DWaveConfig   `yaml:"dwave"`
}

type LocalConfig struct {
	Shots int `yaml:"shots"`
}

// Extra holds provider parameters that don't have a typed field yet — e.g. a
// job option a backend added after this struct was written. Populated from
// the YAML `extra:` map and/or `quell run --set <backend>.<key>=<value>`, and
// merged directly into that backend's outgoing request body (see
// internal/backends/extra.go), so a new provider parameter can be sent
// without a code change as long as it belongs in the same request the
// existing typed fields already populate.
type IBMConfig struct {
	Token    string            `yaml:"token"`
	Instance string            `yaml:"instance"`
	Device   string            `yaml:"device"`
	Shots    int               `yaml:"shots"`
	Extra    map[string]string `yaml:"extra"`
}

// AccessKeyID/SecretAccessKey/SessionToken are optional — when unset,
// RunBraket falls back to the standard AWS_ACCESS_KEY_ID/
// AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN env vars, matching the AWS CLI.
// They exist as config fields (not just env vars) because a multi-tenant
// host running Quell as a service has one process for many orgs — env vars
// can't carry a different credential per caller, so a hosted caller must be
// able to pass them explicitly per request/config instead.
type AWSConfig struct {
	AccessKeyID     string            `yaml:"access_key_id"`
	SecretAccessKey string            `yaml:"secret_access_key"`
	SessionToken    string            `yaml:"session_token"`
	Region          string            `yaml:"region"`
	Device          string            `yaml:"device"`
	S3Bucket        string            `yaml:"s3_bucket"`
	S3Prefix        string            `yaml:"s3_prefix"`
	Shots           int               `yaml:"shots"`
	Extra           map[string]string `yaml:"extra"`
}

type GCPConfig struct {
	Project   string            `yaml:"project"`
	Processor string            `yaml:"processor"`
	Shots     int               `yaml:"shots"`
	KeyFile   string            `yaml:"key_file"` // path to service account JSON, OR the raw JSON content itself (a hosted caller with no local filesystem per org can pass the key contents directly — see googleAccessToken)
	Extra     map[string]string `yaml:"extra"`
}

type RigettiConfig struct {
	APIKey string            `yaml:"api_key"`
	Device string            `yaml:"device"`
	Shots  int               `yaml:"shots"`
	Extra  map[string]string `yaml:"extra"`
}

type IonQConfig struct {
	APIKey string            `yaml:"api_key"`
	Device string            `yaml:"device"`
	Shots  int               `yaml:"shots"`
	Extra  map[string]string `yaml:"extra"`
}

type AzureConfig struct {
	TenantID       string            `yaml:"tenant_id"`
	ClientID       string            `yaml:"client_id"`
	ClientSecret   string            `yaml:"client_secret"`
	SubscriptionID string            `yaml:"subscription_id"`
	ResourceGroup  string            `yaml:"resource_group"`
	Workspace      string            `yaml:"workspace"`
	Target         string            `yaml:"target"`
	Shots          int               `yaml:"shots"`
	Extra          map[string]string `yaml:"extra"`
}

type DWaveConfig struct {
	APIToken string            `yaml:"api_token"`
	Solver   string            `yaml:"solver"`
	Shots    int               `yaml:"shots"`
	Extra    map[string]string `yaml:"extra"`
}

var envVarRe = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand ${ENV_VAR} before parsing
	expanded := envVarRe.ReplaceAllStringFunc(string(data), func(m string) string {
		key := envVarRe.FindStringSubmatch(m)[1]
		if v := os.Getenv(key); v != "" {
			return v
		}
		return m
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	if cfg.Backend == "" {
		cfg.Backend = "local"
	}
	if cfg.Local.Shots == 0 {
		cfg.Local.Shots = 1024
	}
	return &cfg, nil
}

func Default() *Config {
	return &Config{
		Backend: "local",
		Local:   LocalConfig{Shots: 1024},
	}
}

// ExtraFor returns the Extra map for the named backend, initializing it if
// necessary, for `quell run --set <backend>.<key>=<value>` to write into.
// Returns an error for a name that isn't one of the known backends — this
// still requires a backend Quell already has an adapter for; it does not
// let a caller invent an entirely new provider from the command line.
func (c *Config) ExtraFor(backend string) (map[string]string, error) {
	m := map[string]*map[string]string{
		"local":   nil, // local has no request body to extend
		"ibm":     &c.IBM.Extra,
		"aws":     &c.AWS.Extra,
		"google":  &c.Google.Extra,
		"rigetti": &c.Rigetti.Extra,
		"ionq":    &c.IonQ.Extra,
		"azure":   &c.Azure.Extra,
		"dwave":   &c.DWave.Extra,
	}
	p, ok := m[backend]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q for --set — valid: ibm, aws, google, rigetti, ionq, azure, dwave", backend)
	}
	if p == nil {
		return nil, fmt.Errorf("backend %q has no request parameters to extend", backend)
	}
	if *p == nil {
		*p = make(map[string]string)
	}
	return *p, nil
}
