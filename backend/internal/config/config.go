package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func Load(filenames ...string) error {
	if err := godotenv.Load(filenames...); err != nil {
		log.Println("No .env file found")
	}

	// Read required variables from .env.example
	requiredVars, err := readRequiredVarsFromEnvExampleFile()
	if err != nil {
		return fmt.Errorf("could not read required variables from .env.example: %v", err)
	}

	// Check if all required variables are set
	missingVars := []string{}
	println("check vars!")
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			missingVars = append(missingVars, v)
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	log.Println("Configuration loaded")
	return nil
}

// getEnvAsInt reads an environment variable and returns it as int, or returns defaultValue if not set
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Warning: invalid value for %s, using default %d", key, defaultValue)
		return defaultValue
	}
	return value
}

// readRequiredVarsFromEnvExampleFile reads variable names from .env.example
func readRequiredVarsFromEnvExampleFile() ([]string, error) {
	// Try to locate .env.example relative to the current working directory
	examplePath := filepath.Join(".", ".env.example")

	// If not found, check in the parent directory (backend folder)
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		examplePath = filepath.Join("..", ".env.example")
	}

	file, err := os.Open(examplePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .env.example: %v", err)
	}
	defer file.Close()

	var vars []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract variable name (part before the = sign)
		if parts := strings.SplitN(line, "=", 2); len(parts) > 0 {
			varName := strings.TrimSpace(parts[0])
			if varName != "" {
				vars = append(vars, varName)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env.example: %v", err)
	}

	return vars, nil
}
