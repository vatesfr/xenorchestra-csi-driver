/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package xenorchestracsi

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"

	"k8s.io/klog/v2"
)

var stopOnce sync.Once

// log is a package-level logger so the interceptor and its tests can swap the
// destination. Default: klog.
var log = klog.Background()

// loggingInterceptor logs every incoming RPC with the request secrets
// stripped (CSI spec: "the CO and SP MUST NOT log secret values").
// Per-handler request logging is forbidden by golangci-lint (forbidigo) — use this.
func loggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	// "request" key is safe here: the value is sanitized (nolint for the interceptor itself)
	log.V(5).Info(info.FullMethod, "request", protosanitizer.StripSecrets(req)) //nolint:forbidigo
	resp, err := handler(ctx, req)
	log.V(5).Info(info.FullMethod, "error", err, "duration", time.Since(start).String())
	return resp, err
}

// NonBlockingGRPCServer defines non-blocking GRPC server interfaces.
type NonBlockingGRPCServer interface {
	// Start services at the endpoint.
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer)

	// Stop stops the gRPC server. It immediately closes all open connections
	// and listeners. It cancels all active RPCs on the server side and the
	// corresponding pending RPCs on the client side will get notified by
	// connection errors.
	Stop()

	// GracefulStop stops the gRPC server gracefully. It stops the server
	// from accepting new connections and RPCs and blocks until all the
	// pending RPCs are finished.
	GracefulStop()
}

// NewNonBlockingGRPCServer returns an instance of nonBlockingGRPCServer.
func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// nonBlockingGRPCServer implements the interface NonBlockingGRPCServer.
type nonBlockingGRPCServer struct {
	server *grpc.Server
}

// Start implements NonBlockingGRPCServer.
func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	if err := s.serve(endpoint, ids, cs, ns); err != nil {
		klog.Errorf("failed to start grpc server. Err: %v", err)
	}
}

// GracefulStop implements NonBlockingGRPCServer.
func (s *nonBlockingGRPCServer) GracefulStop() {
	stopOnce.Do(func() {
		if s.server != nil {
			s.server.GracefulStop()
		}
		klog.Info("gracefully stopped")
	})
}

// Stop implements NonBlockingGRPCServer.
func (s *nonBlockingGRPCServer) Stop() {
	stopOnce.Do(func() {
		if s.server != nil {
			s.server.Stop()
		}
		klog.Info("stopped")
	})
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer,
	cs csi.ControllerServer, ns csi.NodeServer,
) error {
	const (
		unixScheme = "unix"
		unixPrefix = unixScheme + "://"
	)

	// CSI driver currently supports only unix path.
	if !strings.HasPrefix(endpoint, unixPrefix) {
		err := fmt.Errorf("endpoint must be a unix socket: %s", endpoint)
		klog.Errorf("%s", err.Error())
		return err
	}
	addr := strings.TrimPrefix(endpoint, unixPrefix)

	// Remove UNIX sock file if present
	// NOTE: could also be cleaned at server shutdown.
	if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("failed to remove %s, error: %v", addr, err)
		klog.Errorf("%s", err.Error())
		return err
	}

	listener, err := net.Listen(unixScheme, addr)
	if err != nil {
		err := fmt.Errorf("failed to listen on unix socket %s, error: %v", addr, err)
		klog.Errorf("%s", err.Error())
		return err
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(loggingInterceptor))
	s.server = server

	// Register the CSI services.
	// Always require the identity service.
	if ids == nil {
		err = fmt.Errorf("identity service is required")
		klog.Errorf("%s", err.Error())
		return err
	}

	// Always register the identity service.
	csi.RegisterIdentityServer(s.server, ids)
	klog.Info("identity service registered")

	if cs == nil && ns == nil {
		err = fmt.Errorf("either a controller or node service is required")
		klog.Errorf("%s", err.Error())
		return err
	}

	if cs != nil {
		csi.RegisterControllerServer(s.server, cs)
		klog.Info("controller service registered")
	}
	if ns != nil {
		csi.RegisterNodeServer(s.server, ns)
		klog.Info("node service registered")
	}

	klog.Infof("Listening for connections on address: %#v", listener.Addr())

	if err := server.Serve(listener); err != nil {
		klog.Errorf("failed to serve: %v", err)
		return err
	}

	return nil
}
