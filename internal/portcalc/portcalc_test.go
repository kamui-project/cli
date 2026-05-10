package portcalc

import "testing"

func TestPort_Deterministic(t *testing.T) {
	uuid := "139f862d-a5aa-411b-a1be-d5ce9b8efca4"
	a := Port(uuid)
	b := Port(uuid)
	if a != b {
		t.Fatalf("Port not deterministic: got %d then %d", a, b)
	}
}

func TestPort_InRange(t *testing.T) {
	uuids := []string{
		"00000000-0000-0000-0000-000000000000",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
		"139f862d-a5aa-411b-a1be-d5ce9b8efca4",
		"550e8400-e29b-41d4-a716-446655440000",
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	}
	for _, u := range uuids {
		p := Port(u)
		if p < Lo || p > Hi {
			t.Errorf("Port(%q) = %d, want in [%d, %d]", u, p, Lo, Hi)
		}
	}
}

// TestPort_KnownValues is a sanity guard: if the formula or hash function ever
// changes, this test fails so we notice that dashboard-backend's
// service/project/project_service.go:externalDBLocalPort would also need to
// be updated to match. Values were derived from the actual FNV-1a 32-bit
// (Go stdlib hash/fnv) implementation.
func TestPort_KnownValues(t *testing.T) {
	cases := []struct {
		uuid string
		want int
	}{
		{"", 24005},
		{"139f862d-a5aa-411b-a1be-d5ce9b8efca4", 20262},
	}
	for _, tc := range cases {
		got := Port(tc.uuid)
		if got != tc.want {
			t.Errorf("Port(%q) = %d, want %d (formula regression)", tc.uuid, got, tc.want)
		}
	}
}
