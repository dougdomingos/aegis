package auth

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// LabKeyLength is the required length, in bytes, of the shared lab key.
const LabKeyLength = 32

// ReadLabKey reads the shared lab key from the file at the provided path. The
// file must hold a hex-encoded value that decodes to exactly LabKeyLength
// bytes. Surrounding whitespace is ignored and uppercase hex is accepted.
func ReadLabKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("key file must be hex-encoded: %w", err)
	}

	if len(key) != LabKeyLength {
		return nil, fmt.Errorf("key file must decode to exactly %d bytes", LabKeyLength)
	}

	return key, nil
}
