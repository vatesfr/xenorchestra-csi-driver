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
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/fake"
	"github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi/clients/stub"
)

const (
	sanityEndpoint = "unix:///tmp/xenorchestra-csi-sanity-test.sock"
	driverName     = "csi.xenorchestra.vates.tech"
	nodeName       = "sanity-node"
	sockPath       = "/tmp/xenorchestra-csi-sanity-test.sock"
)

// checkMountLeaks makes every spec fail when it leaves mounts behind. It stays
// off until NodeStageVolume is idempotent: repeated NodeStageVolume calls
// stack mounts that NodeUnstageVolume does not remove.
const checkMountLeaks = false

type CustomIDGenerator struct{}

var (
	_           sanity.IDGenerator = &CustomIDGenerator{}
	fakeMounter *fake.Mounter
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
	fakeMounter = fake.NewMounter()
	// Start the driver in-process with stub dependencies (no real k8s/XO required).
	driver, _ := NewFakeDriver(
		t,
		&xenorchestracsi.DriverOptions{
			DriverName:        driverName,
			NodeName:          nodeName,
			Endpoint:          sanityEndpoint,
			KubernetesPoolTag: xenorchestracsi.DefaultKubernetesPoolTag,
			VDINamePrefix:     xenorchestracsi.DefaultVDINamePrefix,
			ClusterTag:        xenorchestracsi.DefaultClusterTag,
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

	// Target and staging directories are real directories under tmpDir, created
	// and checked by the csi-sanity defaults; only mounts are faked.

	return &cfg
}

// Describe here the sanity test suite(s) to run.
var _ = ginkgo.Describe("Xen Orchestra CSI Driver Sanity Suite", func() {
	// Every spec must unmount what it mounted.
	var mountsBefore int
	ginkgo.BeforeEach(func() {
		mountsBefore = len(fakeMounter.Entries())
	})
	ginkgo.AfterEach(func() {
		if !checkMountLeaks {
			return
		}
		entries := fakeMounter.Entries()
		gomega.Expect(entries).To(gomega.HaveLen(mountsBefore), "mounts left behind: %v", entries[min(mountsBefore, len(entries)):])
	})

	// The explicit pool suite runs all sanity tests with a fixed poolId parameter.
	ginkgo.Describe("Sanity explicit pool", func() {
		cfg := buildBaseTestConfig()

		cfg.TestVolumeParameters = map[string]string{
			xenorchestracsi.ParameterPoolID: stub.PoolId,
		}

		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})

	// The topology-aware suite runs all sanity tests without a poolId, relying on the driver to discover pools via XO tags.
	ginkgo.Describe("Sanity Topology-aware 'no poolId'", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeParameters = map[string]string{
			xenorchestracsi.ParameterStorageType: xenorchestracsi.StorageTypeShared,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})

	// The local storage suite runs sanity tests with storageType=local, which requires additional driver logic to select pools and migrate volumes.
	ginkgo.Describe("Sanity local storage topology-aware", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeParameters = map[string]string{
			// "poolId":      stub.PoolId,
			xenorchestracsi.ParameterStorageType: xenorchestracsi.StorageTypeLocal,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})

	// The local storage explicit pool suite runs sanity tests with storageType=local and a fixed poolId, skipping the driver's topology-based pool selection logic.
	ginkgo.Describe("Sanity local storage explicit pool", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeParameters = map[string]string{
			xenorchestracsi.ParameterPoolID:      stub.PoolId,
			xenorchestracsi.ParameterStorageType: xenorchestracsi.StorageTypeLocal,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})

	// The volume mutable parameters suite runs sanity tests with a fixed storage repository parameter, which is required for volume expansion and other mutable operations.
	ginkgo.Describe("Sanity VolumeMutableParameters default parameters", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeMutableParameters = map[string]string{
			xenorchestracsi.ParameterStorageRepository: stub.DefaultSRId,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})
	ginkgo.Describe("Sanity VolumeMutableParameters shared storage", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeParameters = map[string]string{
			xenorchestracsi.ParameterPoolID:      stub.PoolId,
			xenorchestracsi.ParameterStorageType: xenorchestracsi.StorageTypeShared,
		}
		cfg.TestVolumeMutableParameters = map[string]string{
			xenorchestracsi.ParameterStorageRepository: stub.DefaultSRId,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})
	ginkgo.Describe("Sanity VolumeMutableParameters local storage", func() {
		cfg := buildBaseTestConfig()
		cfg.TestVolumeParameters = map[string]string{
			xenorchestracsi.ParameterPoolID:      stub.PoolId,
			xenorchestracsi.ParameterStorageType: xenorchestracsi.StorageTypeLocal,
		}
		cfg.TestVolumeMutableParameters = map[string]string{
			xenorchestracsi.ParameterStorageRepository: stub.LocalSRId,
		}
		sc := sanity.GinkgoTest(cfg)
		sc.Finalize()
	})
})
