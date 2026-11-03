//go:build ruleguard

// Package gorules holds go-ruleguard rules run by golangci-lint (gocritic).
package gorules

import (
	"strings"

	"github.com/quasilyte/go-ruleguard/dsl"
)

// isLogger reports whether a receiver is klog, fmt, a logr.Logger or a klog.Verbose.
func isLogger(ctx *dsl.VarFilterContext) bool {
	t := ctx.Type.String()
	return t == "github.com/go-logr/logr.Logger" || t == "k8s.io/klog/v2.Verbose"
}

// isCSIMessage reports whether a value is a raw CSI message, e.g. *csi.CreateVolumeRequest.
func isCSIMessage(ctx *dsl.VarFilterContext) bool {
	return strings.HasPrefix(ctx.Type.String(), "*github.com/container-storage-interface/spec/lib/go/csi.")
}

// rawCSIMessageLogged forbids passing a raw CSI message to a logger or formatter:
// CSI requests carry `secrets` maps, and the CSI spec says "the CO and SP MUST NOT
// log secret values". Wrap it with protosanitizer.StripSecrets, or rely on the
// loggingInterceptor in pkg/xenorchestra-csi/server.go.
func rawCSIMessageLogged(m dsl.Matcher) {
	// $*_ binds without backtracking, so each argument position needs its own pattern.
	m.Match(
		`$recv.$_($x, $*_)`,
		`$recv.$_($_, $x, $*_)`,
		`$recv.$_($_, $_, $x, $*_)`,
		`$recv.$_($_, $_, $_, $x, $*_)`,
		`$recv.$_($_, $_, $_, $_, $x, $*_)`,
		`$recv.$_($_, $_, $_, $_, $_, $x, $*_)`,
		`$recv.$_($_, $_, $_, $_, $_, $_, $x, $*_)`,
		`$recv.$_($_, $_, $_, $_, $_, $_, $_, $x, $*_)`,
	).
		Where((m["recv"].Text.Matches(`^(klog|fmt)$`) || m["recv"].Filter(isLogger)) &&
			m["x"].Filter(isCSIMessage)).
		Report("never log raw CSI messages (secrets leak) — wrap with protosanitizer.StripSecrets").
		At(m["x"])
}
