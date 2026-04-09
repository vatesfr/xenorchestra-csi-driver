package sanity_test

import (
	"os"
	"testing"
	"time"

	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	xenorchestracsi "github.com/vatesfr/xenorchestra-csi-driver/pkg/xenorchestra-csi"
)

const (
	sanityEndpoint = "unix:///tmp/xenorchestra-csi-sanity-test.sock"
	driverName     = "csi.xenorchestra.vates.tech"
	nodeName       = "sanity-node"
)

// skipPatterns lists test descriptions to skip because they require features not yet implemented
// (CreateVolume, ValidateVolumeCapabilities).
var skipPatterns = []string{
	"should remove target path",
	"NodeStageVolume.*no volume capability is provided",
	"Node Service should work",
	"Node Service should be idempotent",
	"ValidateVolumeCapabilities",
	"ControllerPublishVolume.*should fail when the volume does not exist",
	"ControllerPublishVolume.*should fail when the node does not exist",
	"volume lifecycle",
}

func TestSanity(t *testing.T) {
	// Start the driver in-process with stub dependencies (no real k8s/XO required).
	driver := xenorchestracsi.NewStubDriver(&xenorchestracsi.DriverOptions{
		DriverName: driverName,
		NodeName:   nodeName,
		Endpoint:   sanityEndpoint,
	})

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

	t.Cleanup(func() {
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

	sc := sanity.GinkgoTest(&cfg)

	gomega.RegisterFailHandler(ginkgo.Fail)

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.SkipStrings = skipPatterns

	ginkgo.RunSpecs(t, "CSI Driver Sanity Suite", suiteConfig, reporterConfig)
	sc.Finalize()
}
