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
	sockPath       = "/tmp/xenorchestra-csi-sanity-test.sock"
)

type CustomIDGenerator struct{}

var (
	_           sanity.IDGenerator = &CustomIDGenerator{}
	fakeMounter *FakeMounter
	tmpDir      string
)

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
	fakeMounter = NewFakeMounter()
	// Start the driver in-process with stub dependencies (no real k8s/XO required).
	driver, _ := NewFakeDriver(
		t,
		&xenorchestracsi.DriverOptions{
			DriverName:         driverName,
			NodeName:           nodeName,
			Endpoint:           sanityEndpoint,
			KubernetesPoolTag:  xenorchestracsi.DefaultKubernetesPoolTag,
			NodeMetadataSource: xenorchestracsi.NodeMetadataSourceKubernetes,
			VDINamePrefix:      xenorchestracsi.DefaultVDINamePrefix,
			ClusterTag:         xenorchestracsi.DefaultClusterTag,
		},
		fakeMounter)

	go func() {
		if err := driver.Run(t.Context()); err != nil {
			t.Errorf("driver.Run: %v", err)
		}
	}()

	// Wait for the driver to be ready: socket must exist and accept connections.
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
	var err error
	tmpDir, err = os.MkdirTemp("", "csi-sanity-*")
	if err != nil {
		t.Fatalf("Failed to create sanity temp working dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
		os.Remove(sockPath)
	})

	gomega.RegisterFailHandler(ginkgo.Fail)

	ginkgo.RunSpecs(t, "CSI Driver Sanity Suite")
}

func buildBaseTestConfig() *sanity.TestConfig {
	// Register sanity tests then run them with a custom SuiteConfig to skip
	// tests that require unimplemented features (CreateVolume, etc.).
	cfg := sanity.NewTestConfig()
	cfg.Address = sanityEndpoint
	cfg.DialOptions = []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("localhost"),
	}
	cfg.ControllerDialOptions = cfg.DialOptions

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

	return &cfg
}

// Describe here the sanity test suite(s) to run.
var _ = ginkgo.Describe("Xen Orchestra CSI Driver Sanity Suite", func() {
	// The explicit pool suite runs all sanity tests with a fixed poolId parameter.
	ginkgo.Describe("Sanity explicit pool", func() {
		cfg := buildBaseTestConfig()

		cfg.TestVolumeParameters = map[string]string{
			"poolId": stub.PoolId,
		}

		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})

	// The topology-aware suite runs all sanity tests without a poolId, relying on the driver to discover pools via XO tags.
	ginkgo.Describe("Sanity Topology-aware 'no poolId'", func() {
		cfg := buildBaseTestConfig()
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})
})
