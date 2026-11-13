// Package security holds the guardrails that keep ProdShadow from touching
// anything but the disposable databases it creates: target address checks,
// ephemeral credential generation, redaction and identifier hashing.
package security

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// ErrUnmanagedTarget is returned for any address that is not a local unix
// socket or a loopback address.
var ErrUnmanagedTarget = errors.New("refusing to connect to an address that is not a local disposable server")

// CheckLocalAddress fails closed unless host is an absolute unix socket
// directory or a loopback IP literal. Host names, including "localhost",
// are rejected because their resolution can be redirected.
func CheckLocalAddress(host string) error {
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrUnmanagedTarget)
	}
	if strings.HasPrefix(host, "/") {
		if filepath.Clean(host) != host {
			return fmt.Errorf("%w: socket directory %q is not a clean absolute path", ErrUnmanagedTarget, host)
		}
		return nil
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return fmt.Errorf("%w: %q is not an IP literal or socket directory", ErrUnmanagedTarget, host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("%w: %s is not a loopback address", ErrUnmanagedTarget, ip)
	}
	return nil
}
