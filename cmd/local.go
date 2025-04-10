package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// localCmd run local server
var localCmd = &cobra.Command{
	Use:   "local",
	Short: "Local broker",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("invalid command")
		}
		return runLocal()
	},
}

var path string
var factor string

func init() {
	localCmd.Flags().StringVar(&path, "path", ".", "Path to local app")
	localCmd.Flags().StringVar(&factor, "factor", "factor info", "factor info command")
	cmd.AddCommand(localCmd)
}
func updateEnv(path string, vars map[string]*string) error {
	envPath := filepath.Join(path, ".env")
	content := make(map[string]string)

	// Read and parse the existing .env file
	if data, err := os.Open(envPath); err == nil {
		defer data.Close()
		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				if unquotedValue, err := strconv.Unquote(value); err == nil {
					content[key] = unquotedValue
				} else {
					content[key] = value // Keep the raw value if unquoting fails
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading .env file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err // Return any error except "file does not exist"
	}

	// Update the content with provided vars
	for k, v := range vars {
		if v != nil {
			content[k] = *v
		}
	}

	// Write to a temp file for atomic update
	tempFile, err := os.CreateTemp(os.TempDir(), "env")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	defer tempFile.Close()

	// Write updated content to the temp file
	for k, v := range content {
		quotedValue := strconv.Quote(v)
		if _, err := tempFile.WriteString(fmt.Sprintf("%s=%s\n", k, quotedValue)); err != nil {
			return err
		}
	}

	if err := tempFile.Sync(); err != nil {
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	// Replace the old .env file with the new one
	if err := os.Rename(tempPath, envPath); err != nil {
		return err
	}

	return nil
}

func getResourceLocal(path string, factor string) *Resource {
	// Run factor meta command and capture output
	args := strings.Fields(factor)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error running factor command: %v\n", err)
		return nil
	}
	name, url, iss, sub := parseMetadata(output)

	updater := func(name string, vars map[string]*string) error {
		return updateEnv(path, vars)
	}
	return getResource(name, url, iss, sub, updater)
}

func runLocal() error {
	if p, ok := os.LookupEnv("APP_PATH"); ok && path == "" {
		path = p
	}
	r := getResourceLocal(path, factor)
	if r == nil {
		return fmt.Errorf("failed to get resource")
	}
	resources := map[string]*Resource{r.ID: r}
	return runBrokerServer("8003", resources)
}
