package nftables

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"dougdomingos.com/aegis/internal/sentinel/filtering"
)

// defaultBinaryPath is the nft CLI executable used when none is configured.
const defaultBinaryPath = "nft"

// runFunc executes a command with the provided arguments and standard input.
type runFunc func(binary string, args []string, stdin string) error

// Executor programs the Aegis nftables table through the nft CLI.
type Executor struct {
	binaryPath string
	run        runFunc
}

// NewExecutor builds an Executor that invokes binaryPath, defaulting to nft.
func NewExecutor(binaryPath string) *Executor {
	if binaryPath == "" {
		binaryPath = defaultBinaryPath
	}

	return &Executor{
		binaryPath: binaryPath,
		run:        runNftCommand,
	}
}

// Apply replaces the Aegis ruleset with the rendered policy snapshot.
func (e *Executor) Apply(snapshot filtering.PolicySnapshot) error {
	return e.execute([]string{"-f"}, RenderRuleset(snapshot))
}

// Clear removes the Aegis nftables table.
func (e *Executor) Clear() error {
	return e.execute([]string{"-f"}, RenderClear())
}

// AddResolvedIP adds a validated DNS answer address to its address-family set.
func (e *Executor) AddResolvedIP(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid resolved IP address %q", ip)
	}

	setName := ResolvedSetName
	if strings.Contains(ip, ":") {
		setName = ResolvedSetName6
	}

	args := []string{"add", "element", "inet", "aegis", setName, "{", ip, "}"}
	return e.execute(args, "")
}

// execute runs one nft command and wraps failures with the command context.
func (e *Executor) execute(args []string, stdin string) error {
	if err := e.run(e.binaryPath, args, stdin); err != nil {
		command := strings.Join(append([]string{e.binaryPath}, args...), " ")
		return fmt.Errorf("execute nft command %q: %w", command, err)
	}

	return nil
}

// runNftCommand executes the nft CLI with the supplied standard input.
func runNftCommand(binary string, args []string, stdin string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = strings.NewReader(stdin)

	return cmd.Run()
}
