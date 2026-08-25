package config

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSystemCredentialUsesOnlyKeyring(t *testing.T) {
	originalSet, originalGet, originalDelete := systemKeyringSet, systemKeyringGet, systemKeyringDelete
	t.Cleanup(func() {
		systemKeyringSet, systemKeyringGet, systemKeyringDelete = originalSet, originalGet, originalDelete
	})
	stored := map[string]string{}
	systemKeyringSet = func(service, account, value string) error {
		if service != systemCredentialService {
			t.Fatalf("service = %q", service)
		}
		stored[account] = value
		return nil
	}
	systemKeyringGet = func(service, account string) (string, error) {
		value, ok := stored[account]
		if !ok {
			return "", keyring.ErrNotFound
		}
		return value, nil
	}
	systemKeyringDelete = func(service, account string) error { delete(stored, account); return nil }

	if err := SetSystemCredential("github", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if value, err := GetSystemCredential("github"); err != nil || value != "secret-token" {
		t.Fatalf("get = %q, %v", value, err)
	}
	if err := DeleteSystemCredential("github"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSystemCredential("github"); !errors.Is(err, ErrSystemCredentialNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestSystemCredentialRejectsUnsafeValues(t *testing.T) {
	if err := SetSystemCredential("", "token"); err == nil {
		t.Fatal("empty account accepted")
	}
	if err := SetSystemCredential("github", "line1\nline2"); err == nil {
		t.Fatal("newline credential accepted")
	}
}

func TestProviderCredentialUsesNamespacedSystemAccount(t *testing.T) {
	originalSet, originalGet, originalDelete := systemKeyringSet, systemKeyringGet, systemKeyringDelete
	t.Cleanup(func() {
		systemKeyringSet, systemKeyringGet, systemKeyringDelete = originalSet, originalGet, originalDelete
	})
	stored := map[string]string{}
	systemKeyringSet = func(service, account, value string) error {
		if service != systemCredentialService || account != providerCredentialAccountPrefix+"TUZI_API_KEY" {
			t.Fatalf("unexpected keyring target %q/%q", service, account)
		}
		stored[account] = value
		return nil
	}
	systemKeyringGet = func(_, account string) (string, error) {
		value, ok := stored[account]
		if !ok {
			return "", keyring.ErrNotFound
		}
		return value, nil
	}
	systemKeyringDelete = func(_, account string) error { delete(stored, account); return nil }

	before := ProviderCredentialGeneration()
	if err := SetProviderCredential("TUZI_API_KEY", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if value, err := GetProviderCredential("TUZI_API_KEY"); err != nil || value != "secret-token" {
		t.Fatalf("get provider credential = %q, %v", value, err)
	}
	if ProviderCredentialGeneration() <= before {
		t.Fatal("provider credential generation did not advance after save")
	}
	if err := DeleteProviderCredential("TUZI_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetProviderCredential("TUZI_API_KEY"); !errors.Is(err, ErrSystemCredentialNotFound) {
		t.Fatalf("deleted provider credential error = %v", err)
	}
}
