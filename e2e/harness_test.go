//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/schemas"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// networkSubnet is the fixed IPv4 subnet of the fleet network. The
	// addresses inside it are planned, so the accepted-machines registry can
	// be rendered before any container starts.
	networkSubnet = "172.30.0.0/24"

	// orchestratorIP is the planned static address of the Orchestrator.
	orchestratorIP = "172.30.0.10"

	// orchestratorPort is the container port the Orchestrator listens on.
	orchestratorPort = "8080/tcp"

	// validLabKeyHex is a 32-byte lab key in lowercase hexadecimal shared by
	// the Orchestrator and the fleet in the happy-path scenarios.
	validLabKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// wrongLabKeyHex is a different 32-byte lab key used to invalidate the
	// Sentinel signature on purpose.
	wrongLabKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	// sessionSecret is the HMAC signing secret given to the Orchestrator.
	sessionSecret = "e2e-session-secret"

	// labKeyPath is the in-container path of the shared lab key file.
	labKeyPath = "/etc/aegis/lab.key"

	// machinesFilePath is the in-container path of the accepted-machines CSV.
	machinesFilePath = "/etc/aegis/machines.csv"

	// sentinelConfigPath is the in-container path of the Sentinel JSON config.
	sentinelConfigPath = "/etc/aegis/sentinel.json"

	// readyTimeout caps how long a fleet member may take to reach its expected
	// state.
	readyTimeout = 60 * time.Second

	// imageRepo and imageTag name the image built once per suite run.
	imageRepo = "aegis-e2e"
	imageTag  = "test"
)

// e2eImage is the reference (repo:tag) of the image built by TestMain.
var e2eImage string

// TestMain builds the single Aegis image once and runs the suite.
func TestMain(m *testing.M) {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to create the docker provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	ref, err := provider.BuildImage(context.Background(), &testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "..",
			Dockerfile: "e2e/Dockerfile",
			Repo:       imageRepo,
			Tag:        imageTag,
			KeepImage:  true,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to build the aegis image: %v\n", err)
		os.Exit(1)
	}

	e2eImage = ref

	os.Exit(m.Run())
}

// sentinelIP returns the planned static IP of the sentinel container at index.
func sentinelIP(index int) string {
	return fmt.Sprintf("172.30.0.%d", 11+index)
}

// machineName returns the registry name assigned to the machine at index.
func machineName(index int) string {
	return fmt.Sprintf("lcc1-%03d", index+1)
}

// fleet is a per-test Aegis network on the fixed subnet plus its Orchestrator.
// The accepted-machines registry is rendered from the planned IPs before the
// Orchestrator starts, since SyncMachines runs only once at startup.
type fleet struct {
	t            *testing.T
	network      *testcontainers.DockerNetwork
	machineCount int
	orchestrator testcontainers.Container
}

// newFleet creates the fleet network with the fixed IPAM subnet and registers
// its cleanup.
func newFleet(t *testing.T, machineCount int) *fleet {
	t.Helper()

	nw, err := tcnetwork.New(context.Background(),
		tcnetwork.WithDriver("bridge"),
		tcnetwork.WithIPAM(&mobynetwork.IPAM{
			Driver: "default",
			Config: []mobynetwork.IPAMConfig{{Subnet: netip.MustParsePrefix(networkSubnet)}},
		}),
	)
	if err != nil {
		t.Fatalf("failed to create the fleet network: %v", err)
	}

	testcontainers.CleanupNetwork(t, nw)

	return &fleet{t: t, network: nw, machineCount: machineCount}
}

// startOrchestrator boots the real Orchestrator container on the fleet network
// with the planned static IP, the injected lab key and machines registry, and
// its API port published to the host, waiting for it to serve /machines.
func (f *fleet) startOrchestrator(t *testing.T) testcontainers.Container {
	t.Helper()

	registry := "ip,name\n"
	for i := 0; i < f.machineCount; i++ {
		registry += fmt.Sprintf("%s,%s\n", sentinelIP(i), machineName(i))
	}

	c, err := testcontainers.Run(context.Background(), e2eImage,
		testcontainers.WithEnv(map[string]string{
			"AEGIS_LAB_KEY":        labKeyPath,
			"AEGIS_SESSION_SECRET": sessionSecret,
			"AEGIS_MACHINES":       machinesFilePath,
			"AEGIS_DB":             "/home/nonroot/aegis.db",
		}),
		testcontainers.WithFiles(
			injectedFile(labKeyPath, validLabKeyHex+"\n"),
			injectedFile(machinesFilePath, registry),
		),
		testcontainers.WithExposedPorts(orchestratorPort),
		testcontainers.WithWaitStrategyAndDeadline(readyTimeout,
			wait.ForHTTP("/machines").WithPort(orchestratorPort)),
		tcnetwork.WithNetwork(nil, f.network),
		withStaticIP(f.network.Name, orchestratorIP),
	)
	if err != nil {
		t.Fatalf("failed to start the orchestrator: %v", err)
	}

	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Errorf("failed to terminate the orchestrator: %v", err)
		}
	})

	f.orchestrator = c

	return c
}

// machines lists the current machine registry keyed by IP via GET /machines on
// the host-mapped port of the running Orchestrator.
func (f *fleet) machines(t *testing.T) map[string]schemas.MachineOutputSchema {
	t.Helper()

	ctx := context.Background()

	endpoint, err := f.orchestrator.PortEndpoint(ctx, orchestratorPort, "http")
	if err != nil {
		t.Fatalf("failed to resolve the orchestrator endpoint: %v", err)
	}

	resp, err := http.Get(endpoint + "/machines")
	if err != nil {
		t.Fatalf("failed to list the machines: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list machines status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var list []schemas.MachineOutputSchema
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode the machines payload: %v", err)
	}

	byIP := make(map[string]schemas.MachineOutputSchema, len(list))
	for _, machine := range list {
		byIP[machine.IP] = machine
	}

	return byIP
}

// injectedFile declares an in-memory file copied into the container before it
// starts. The mode is world-readable because the distroless runtime user is
// nonroot and the containers are ephemeral.
func injectedFile(containerPath, content string) testcontainers.ContainerFile {
	return testcontainers.ContainerFile{
		Reader:            strings.NewReader(content),
		ContainerFilePath: containerPath,
		FileMode:          0o644,
	}
}

// withStaticIP pins the container's address inside the named network, so the
// IP detected by the Sentinel matches the planned entry in the registry.
func withStaticIP(networkName, ip string) testcontainers.CustomizeRequestOption {
	addr := netip.MustParseAddr(ip)

	return testcontainers.WithEndpointSettingsModifier(
		func(settings map[string]*mobynetwork.EndpointSettings) {
			if settings[networkName] == nil {
				settings[networkName] = &mobynetwork.EndpointSettings{}
			}
			settings[networkName].IPAMConfig = &mobynetwork.EndpointIPAMConfig{IPv4Address: addr}
		},
	)
}
