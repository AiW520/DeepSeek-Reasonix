package config

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/zalando/go-keyring"
)

// ErrSystemCredentialNotFound reports an absent system-keyring entry. System
// credentials deliberately never fall back to Reasonix's dotenv store.
var ErrSystemCredentialNotFound = errors.New("system credential not found")

const systemCredentialService = "reasonix"

const providerCredentialAccountPrefix = "provider/"

var (
	systemKeyringSet    = keyring.Set
	systemKeyringGet    = keyring.Get
	systemKeyringDelete = keyring.Delete
)

var providerCredentialGeneration atomic.Uint64

func SetSystemCredential(account, value string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return fmt.Errorf("system credential account is required")
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("system credential value is invalid")
	}
	if err := systemKeyringSet(systemCredentialService, account, value); err != nil {
		return fmt.Errorf("store credential in the operating system keyring: %w", err)
	}
	return nil
}

func GetSystemCredential(account string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return "", fmt.Errorf("system credential account is required")
	}
	value, err := systemKeyringGet(systemCredentialService, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrSystemCredentialNotFound
		}
		return "", fmt.Errorf("read credential from the operating system keyring: %w", err)
	}
	if value == "" {
		return "", ErrSystemCredentialNotFound
	}
	return value, nil
}

func DeleteSystemCredential(account string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil
	}
	if err := systemKeyringDelete(systemCredentialService, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete credential from the operating system keyring: %w", err)
	}
	return nil
}

func SetProviderCredential(apiKeyEnv, value string) error {
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	if apiKeyEnv == "" {
		return fmt.Errorf("provider credential account is required")
	}
	if err := SetSystemCredential(providerCredentialAccountPrefix+apiKeyEnv, value); err != nil {
		return err
	}
	providerCredentialGeneration.Add(1)
	return nil
}

func GetProviderCredential(apiKeyEnv string) (string, error) {
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	if apiKeyEnv == "" {
		return "", ErrSystemCredentialNotFound
	}
	return GetSystemCredential(providerCredentialAccountPrefix + apiKeyEnv)
}

func DeleteProviderCredential(apiKeyEnv string) error {
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	if apiKeyEnv == "" {
		return nil
	}
	if err := DeleteSystemCredential(providerCredentialAccountPrefix + apiKeyEnv); err != nil {
		return err
	}
	providerCredentialGeneration.Add(1)
	return nil
}

func ProviderCredentialGeneration() uint64 {
	return providerCredentialGeneration.Load()
}
