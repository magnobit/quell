package config

import (
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Backend string      `yaml:"backend"`
	Local   LocalConfig `yaml:"local"`
	IBM     IBMConfig   `yaml:"ibm"`
	AWS     AWSConfig   `yaml:"aws"`
	Google  GCPConfig   `yaml:"google"`
	Rigetti RigettiConfig `yaml:"rigetti"`
}

type LocalConfig struct {
	Shots int `yaml:"shots"`
}

type IBMConfig struct {
	Token    string `yaml:"token"`
	Instance string `yaml:"instance"`
	Device   string `yaml:"device"`
	Shots    int    `yaml:"shots"`
}

type AWSConfig struct {
	Region   string `yaml:"region"`
	Device   string `yaml:"device"`
	S3Bucket string `yaml:"s3_bucket"`
	S3Prefix string `yaml:"s3_prefix"`
	Shots    int    `yaml:"shots"`
}

type GCPConfig struct {
	Project   string `yaml:"project"`
	Processor string `yaml:"processor"`
	Shots     int    `yaml:"shots"`
}

type RigettiConfig struct {
	APIKey string `yaml:"api_key"`
	Device string `yaml:"device"`
	Shots  int    `yaml:"shots"`
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
