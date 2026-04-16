package sanity_test

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/stub"
)

const (
	sanityEndpoint = "unix:///tmp/xenorchestra-csi-sanity-test.sock"
	driverName     = "csi.xenorchestra.vates.tech"
	nodeName       = "sanity-node"
)

// skipPatterns lists test descriptions to skip because they require features not yet implemented
// (CreateVolume, ValidateVolumeCapabilities).
var skipPatterns = []string{
	// Unimplemented features:
	"ValidateVolumeCapabilities",
	// Automatically skipped because not supported by the driver:
	// "Snapshot",
	// "ListVolumes",
	// "ExpandVolume",
	// "ModifyVolume",
	// "GroupController",
	// "NodeGetVolumeStats",
}

type CustomIDGenerator struct {
	// Empty struct since no state is needed to generate IDs
}

var _ sanity.IDGenerator = &CustomIDGenerator{}

func (d CustomIDGenerator) GenerateUniqueValidVolumeID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (d CustomIDGenerator) GenerateInvalidVolumeID() string {
	return "fake-vol-id"
}

func (d CustomIDGenerator) GenerateUniqueValidNodeID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (d CustomIDGenerator) GenerateInvalidNodeID() string {
	return "fake-node-id"
}

func TestSanity(t *testing.T) {
	fakeMounter := stub.NewStubMounter()
	// Start the driver in-process with stub dependencies (no real k8s/XO required).
	driver, _ := NewFakeDriver(
		t,
		&xenorchestracsi.DriverOptions{
			DriverName: driverName,
			NodeName:   nodeName,
			Endpoint:   sanityEndpoint,
		},
		fakeMounter)

	go func() {
		if err := driver.Run(t.Context()); err != nil {
			t.Errorf("driver.Run: %v", err)
		}
	}()

	// Wait for the driver to be ready: socket must exist and accept connections.
	sockPath := "/tmp/xenorchestra-csi-sanity-test.sock"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			// Give the gRPC server a moment to start accepting after the socket file appears.
			time.Sleep(100 * time.Millisecond)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Use temp dir and cleanup any target paths left behind by failed tests
	tmpDir, err := os.MkdirTemp("", "csi-sanity-*")
	if err != nil {
		t.Fatalf("Failed to create sanity temp working dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
		os.Remove(sockPath)
	})

	// Register sanity tests then run them with a custom SuiteConfig to skip
	// tests that require unimplemented features (CreateVolume, etc.).
	cfg := sanity.NewTestConfig()
	cfg.Address = sanityEndpoint
	cfg.DialOptions = []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("localhost"),
	}
	cfg.ControllerDialOptions = cfg.DialOptions

	cfg.TestVolumeParameters = map[string]string{
		"poolId": stub.PoolId,
	}
	cfg.IDGen = &CustomIDGenerator{}
	cfg.TargetPath = path.Join(tmpDir, "mount")
	cfg.StagingPath = path.Join(tmpDir, "staging")

	// Use the fake mounter's methods to simulate filesystem operations in memory,
	// allowing sanity tests to run without real mounts.
	cfg.CheckPath = func(path string) (sanity.PathKind, error) {
		return fakeMounter.CheckPath(path)
	}
	cfg.CreateTargetDir = func(path string) (string, error) {
		return path, fakeMounter.Mount("", path, "", nil)
	}
	cfg.CreateStagingDir = func(path string) (string, error) {
		return path, fakeMounter.Mount("", path, "", nil)
	}
	cfg.RemoveStagingPath = func(path string) error {
		return fakeMounter.Unmount(path)
	}
	cfg.RemoveTargetPath = func(path string) error {
		return fakeMounter.Unmount(path)
	}
	sc := sanity.GinkgoTest(&cfg)

	gomega.RegisterFailHandler(ginkgo.Fail)

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.SkipStrings = skipPatterns

	ginkgo.RunSpecs(t, "CSI Driver Sanity Suite", suiteConfig, reporterConfig)
	sc.Finalize()
}
