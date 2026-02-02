// Package sandbox provides authorization code generation for sensitive operations.
package sandbox

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/stuffbucket/coop/internal/config"
)

const (
	// CodeWindow is the TOTP step size.
	CodeWindow = 30 * time.Second
	// CodeDigits is the number of digits in the authorization code.
	CodeDigits = 6
	// seatbeltKeyFile is where we persist the secret used for host-only TOTP.
	seatbeltKeyFile = "seatbelt.key"
)

var (
	secretOnce sync.Once
	secretVal  string
	secretErr  error
)

// secretPath returns the absolute path to the persisted seatbelt secret.
func secretPath() string {
	dirs := config.GetDirectories()
	return filepath.Join(dirs.Config, seatbeltKeyFile)
}

// loadOrCreateSecret returns the base32 secret, creating it if missing.
func loadOrCreateSecret() (string, error) {
	path := secretPath()

	if data, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s, nil
		}
	}

	buf := make([]byte, 20) // 160-bit secret
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate seatbelt secret: %w", err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write seatbelt secret: %w", err)
	}

	return secret, nil
}

// seatbeltSecret returns a cached per-user secret for TOTP.
func seatbeltSecret() (string, error) {
	secretOnce.Do(func() {
		secretVal, secretErr = loadOrCreateSecret()
	})
	return secretVal, secretErr
}

var totpOpts = totp.ValidateOpts{
	Period:    uint(CodeWindow.Seconds()),
	Skew:      1,
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA256,
}

// CurrentAuthCode returns the current authorization code.
func CurrentAuthCode() (string, error) {
	secret, err := seatbeltSecret()
	if err != nil {
		return "", err
	}
	code, err := totp.GenerateCodeCustom(secret, time.Now(), totpOpts)
	if err != nil {
		return "", err
	}
	return code, nil
}

// ValidateAuthCode checks if the provided code matches current or skewed window.
func ValidateAuthCode(code string) (bool, error) {
	secret, err := seatbeltSecret()
	if err != nil {
		return false, err
	}
	valid, err := totp.ValidateCustom(strings.TrimSpace(code), secret, time.Now(), totpOpts)
	return valid, err
}

// RotateSeatbeltSecret deletes the persisted secret and generates a new one.
func RotateSeatbeltSecret() (string, error) {
	secretOnce = sync.Once{} // reset cache
	path := secretPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove seatbelt secret: %w", err)
	}
	return seatbeltSecret()
}

// ParseForceFlag parses a --force flag value.
// Returns (forceRequested, codeProvided, codeValue)
func ParseForceFlag(value string) (bool, bool, string) {
	if value == "" {
		return false, false, ""
	}

	// Check if it's just "true" or a code
	if value == "true" {
		return true, false, ""
	}

	// Try to parse as a number (the auth code)
	if _, err := strconv.Atoi(value); err == nil && len(value) == CodeDigits {
		return true, true, value
	}

	return true, false, ""
}
