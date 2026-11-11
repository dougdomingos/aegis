package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strings"

	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// machineImportHeader is the expected header row of a machine registry import.
var machineImportHeader = []string{"ip", "name", "lab"}

// parseMachineImport reads the entries of a machine registry import from the
// provided "ip,name,lab" CSV document. Every field is trimmed; the IP must be
// valid and unique within the document, the name must not be empty, and an
// empty lab is allowed.
func parseMachineImport(document io.Reader) ([]domain.MachineImport, error) {
	reader := csv.NewReader(document)
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", machineErrors.ErrInvalidMachinesCSV, err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%w: document is empty", machineErrors.ErrInvalidMachinesCSV)
	}

	if !isMachineImportHeader(records[0]) {
		return nil, fmt.Errorf("%w: header must be %q", machineErrors.ErrInvalidMachinesCSV, strings.Join(machineImportHeader, ","))
	}

	entries := make([]domain.MachineImport, 0, len(records)-1)
	seen := make(map[string]bool, len(records)-1)

	for index, record := range records[1:] {
		line := index + 2

		if len(record) != len(machineImportHeader) {
			return nil, fmt.Errorf("%w: line %d must have %d fields", machineErrors.ErrInvalidMachinesCSV, line, len(machineImportHeader))
		}

		ip := strings.TrimSpace(record[0])
		name := strings.TrimSpace(record[1])
		lab := strings.TrimSpace(record[2])

		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("%w: line %d: %q is not a valid IP address", machineErrors.ErrInvalidMachinesCSV, line, ip)
		}

		if seen[ip] {
			return nil, fmt.Errorf("%w: line %d: duplicate IP address %q", machineErrors.ErrInvalidMachinesCSV, line, ip)
		}

		if name == "" {
			return nil, fmt.Errorf("%w: line %d: empty name", machineErrors.ErrInvalidMachinesCSV, line)
		}

		seen[ip] = true
		entries = append(entries, domain.MachineImport{
			Machine: domain.Machine{IP: ip, Name: name},
			Lab:     lab,
		})
	}

	return entries, nil
}

// isMachineImportHeader reports whether the provided record matches the
// expected header row, ignoring surrounding whitespace.
func isMachineImportHeader(record []string) bool {
	if len(record) != len(machineImportHeader) {
		return false
	}

	for index, field := range record {
		if strings.TrimSpace(field) != machineImportHeader[index] {
			return false
		}
	}

	return true
}
