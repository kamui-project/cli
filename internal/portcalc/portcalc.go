// Package portcalc computes the local TCP port a `cloudflared access tcp`
// listener should bind to for a given database UUID.
//
// This formula MUST stay in sync with dashboard-backend's
// service/project/project_service.go:externalDBLocalPort. Both sides hash the
// database UUID with FNV-1a 32-bit and map to the [16384, 32767] range.
//
//	port = 16384 + (fnv32a(uuid) % 16384)
//
// Picking the same algorithm on both sides lets the dashboard display a port
// users can immediately use in TablePlus / psql, without daemon polling for
// the actual listener address.
package portcalc

import (
	"hash/fnv"
)

const (
	// Lo is the lowest local port the daemon may bind to (16384, IANA
	// "registered" range cutoff). Avoids common ephemeral / well-known ports.
	Lo = 16384
	// Hi is the highest local port the daemon may bind to (32767).
	Hi = 32767
)

// Port returns the deterministic local port for the given database UUID.
// The returned value is always in [Lo, Hi].
func Port(databaseUUID string) int {
	const span = Hi - Lo + 1
	h := fnv.New32a()
	_, _ = h.Write([]byte(databaseUUID))
	return Lo + int(h.Sum32()%uint32(span))
}
