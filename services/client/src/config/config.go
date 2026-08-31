// Package config loads and validates the client process configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config contains all external values required by one client instance.
type Config struct {
	ServerHost string
	ServerPort string
	AgencyID   uint32
	BatchSize  uint32
	InputFile  string
	OutputFile string
}

// requiredEnvironmentVariable returns a non-empty environment value.
func requiredEnvironmentVariable(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s environment variable is required", name)
	}
	return value, nil
}

// loadBatchSize reads and validates the mandatory positive uint32 BATCH_SIZE.
func loadBatchSize() (uint32, error) {
	value, err := requiredEnvironmentVariable("BATCH_SIZE")
	if err != nil {
		return 0, err
	}

	batchSize, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("BATCH_SIZE must be a positive uint32: %w", err)
	}
	if batchSize == 0 {
		return 0, errors.New("BATCH_SIZE must be greater than zero")
	}
	return uint32(batchSize), nil
}

// Load reads the environment once and returns a fully validated configuration.
func Load() (Config, error) {
	agencyIDValue, err := requiredEnvironmentVariable("AGENCY_ID")
	if err != nil {
		return Config{}, err
	}
	agencyID, err := strconv.ParseUint(agencyIDValue, 10, 32)
	if err != nil {
		return Config{}, fmt.Errorf("AGENCY_ID must be a uint32: %w", err)
	}

	serverHost, err := requiredEnvironmentVariable("SERVER_HOST")
	if err != nil {
		return Config{}, err
	}
	serverPort, err := requiredEnvironmentVariable("SERVER_PORT")
	if err != nil {
		return Config{}, err
	}
	inputFile, err := requiredEnvironmentVariable("INPUT_FILE")
	if err != nil {
		return Config{}, err
	}
	outputFile, err := requiredEnvironmentVariable("OUTPUT_FILE")
	if err != nil {
		return Config{}, err
	}
	batchSize, err := loadBatchSize()
	if err != nil {
		return Config{}, err
	}

	return Config{
		ServerHost: serverHost,
		ServerPort: serverPort,
		AgencyID:   uint32(agencyID),
		BatchSize:  batchSize,
		InputFile:  inputFile,
		OutputFile: outputFile,
	}, nil
}
