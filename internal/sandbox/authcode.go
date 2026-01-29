// Package sandbox provides authorization code generation for sensitive operations.
package sandbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// CodeWindow is the time window for each code (15 seconds)
	CodeWindow = 15 * time.Second
	// CodeDigits is the number of digits in the authorization code
	CodeDigits = 6
)

// machineSecret generates a machine-specific secret for TOTP.
// Uses hostname and home directory to create a stable local secret.
func machineSecret() []byte {
	hostname, _ := os.Hostname()
	home := os.Getenv("HOME")
	
	// Combine machine identifiers - this is local-only security,
	// not meant to be cryptographically unguessable by remote attackers,
	// just unpredictable to automated processes
	seed := fmt.Sprintf("coop:%s:%s:seatbelt", hostname, home)
	
	h := sha256.Sum256([]byte(seed))
	return h[:]
}

// generateCode creates a 6-digit code for a given time counter.
func generateCode(secret []byte, counter uint64) string {
	// Convert counter to bytes (big-endian)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	// HMAC-SHA256
	mac := hmac.New(sha256.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation (similar to TOTP RFC 6238)
	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff

	// Get last N digits
	code := truncated % 1000000
	return fmt.Sprintf("%06d", code)
}

// CurrentAuthCode returns the current authorization code.
func CurrentAuthCode() string {
	counter := uint64(time.Now().Unix() / int64(CodeWindow.Seconds()))
	return generateCode(machineSecret(), counter)
}

// PreviousAuthCode returns the previous window's authorization code.
func PreviousAuthCode() string {
	counter := uint64(time.Now().Unix()/int64(CodeWindow.Seconds())) - 1
	return generateCode(machineSecret(), counter)
}

// ValidateAuthCode checks if the provided code matches current or previous window.
func ValidateAuthCode(code string) bool {
	// Normalize to 6 digits
	if len(code) < CodeDigits {
		code = fmt.Sprintf("%06s", code)
	}
	
	current := CurrentAuthCode()
	previous := PreviousAuthCode()
	
	return code == current || code == previous
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
