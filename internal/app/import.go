package app

import (
	"fmt"
	"os"
	"path/filepath"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/converter/nginx"
	"rua.plus/lolly/internal/version"

	"gopkg.in/yaml.v3"
)

// Run decides behavior based on flags: generate config, import nginx config, print version, or start the app.
func Run(cfgPath string, genConfig bool, outputPath string, importPath string, showVersion bool) int {
	if genConfig && importPath != "" {
		fmt.Fprintln(os.Stderr, "error: --generate-config and --import are mutually exclusive")
		return 1
	}
	if outputPath != "" && !genConfig && importPath == "" {
		fmt.Fprintln(os.Stderr, "error: -o requires either --generate-config or --import")
		return 1
	}

	if genConfig {
		return generateConfig(outputPath)
	}

	if importPath != "" {
		if err := importNginxConfig(importPath, outputPath); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0
	}

	if showVersion {
		printVersion()
		return 0
	}

	app := NewApp(cfgPath)
	return app.Run()
}

func generateConfig(outputPath string) int {
	cfg := config.DefaultConfig()
	yamlData, err := config.GenerateConfigYAML(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate config: %v\n", err)
		return 1
	}

	if outputPath == "" {
		fmt.Print(string(yamlData))
	} else {
		if err := os.WriteFile(outputPath, yamlData, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write file: %v\n", err)
			return 1
		}
		fmt.Printf("Config written to: %s\n", outputPath)
	}
	return 0
}

func importNginxConfig(path, outputPath string) error {
	nginxCfg, err := nginx.ParseFile(path)
	if err != nil {
		return fmt.Errorf("failed to parse nginx config: %w", err)
	}

	result, err := nginx.Convert(nginxCfg)
	if err != nil {
		return fmt.Errorf("failed to convert config: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s:line %d: %s\n", w.File, w.Line, w.Message)
	}

	if validateErr := config.Validate(result.Config); validateErr != nil {
		return fmt.Errorf("converted config validation failed: %w", validateErr)
	}

	yamlData, err := yaml.Marshal(result.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if outputPath == "" {
		if _, err := os.Stdout.Write(yamlData); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		if err := os.WriteFile(outputPath, yamlData, 0o644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Config written to: %s\n", outputPath)
	}

	return nil
}

func printVersion() {
	fmt.Printf("lolly version %s\n", version.Version)
	fmt.Printf("  Git: %s (%s)\n", version.GitCommit, version.GitBranch)
	fmt.Printf("  Built: %s\n", version.BuildTime)
	fmt.Printf("  Go: %s\n", version.GoVersion)
	fmt.Printf("  Platform: %s\n", version.BuildPlatform)
}
