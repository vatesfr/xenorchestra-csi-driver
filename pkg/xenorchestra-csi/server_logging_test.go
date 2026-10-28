package xenorchestracsi

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// captureSink is a minimal logr.LogSink that records every message, so the
// test can assert on what the interceptor would have written to the log.
type captureSink struct {
	buf *bytes.Buffer
}

func (s *captureSink) Init(info logr.RuntimeInfo) {}
func (s *captureSink) Enabled(level int) bool     { return true }
func (s *captureSink) Info(level int, msg string, kvList ...any) {
	fmt.Fprintf(s.buf, "INFO %s %v\n", msg, kvList)
}

func (s *captureSink) Error(err error, msg string, kvList ...any) {
	fmt.Fprintf(s.buf, "ERR %s %v\n", msg, kvList)
}
func (s *captureSink) WithValues(kvList ...any) logr.LogSink { return s }
func (s *captureSink) WithName(name string) logr.LogSink     { return s }

func TestLoggingInterceptorStripsSecrets(t *testing.T) {
	buf := &bytes.Buffer{}
	old := log
	log = logr.New(&captureSink{buf: buf})
	t.Cleanup(func() { log = old })

	handler := func(context.Context, any) (any, error) {
		return &csi.CreateVolumeResponse{}, nil
	}

	_, err := loggingInterceptor(context.Background(),
		&csi.CreateVolumeRequest{Name: "vol", Secrets: map[string]string{"token": "abc"}},
		&grpc.UnaryServerInfo{FullMethod: "/csi.v1.Controller/CreateVolume"},
		handler)
	require.NoError(t, err)

	out := buf.String()
	require.NotContains(t, out, "abc", "secret value must never reach the log")
	require.Contains(t, out, "***", "secrets must be redacted, not dropped")
	require.Contains(t, out, "/csi.v1.Controller/CreateVolume")
}
