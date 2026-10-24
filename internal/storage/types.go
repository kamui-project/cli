package storage

import "time"

type Item struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CommittedBytes int64  `json:"committed_bytes"`
	ReservedBytes  int64  `json:"reserved_bytes"`
	ObjectCount    int64  `json:"object_count"`
	Status         string `json:"status"`
}
type Overview struct {
	Endpoint          string `json:"endpoint"`
	RecoveryAvailable bool   `json:"recovery_available"`
	Items             []Item `json:"items"`
	Limits            struct {
		CapacityBytes int64 `json:"capacity_bytes"`
	} `json:"limits"`
	Settings struct {
		OperationBudgetMicroUSD *int64 `json:"operation_budget_micro_usd"`
	} `json:"settings"`
}
type Object struct {
	Key        string `json:"key"`
	Generation string `json:"generation"`
	SizeBytes  int64  `json:"size_bytes"`
}
type ObjectPage struct {
	Items      []Object `json:"items"`
	NextCursor *string  `json:"next_cursor"`
}
type Permit struct {
	Endpoint  string    `json:"endpoint"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	StorageID string    `json:"storage_id"`
	Role      string    `json:"role"`
}
type Upload struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	SizeBytes     int64  `json:"size_bytes"`
	Mode          string `json:"mode"`
	PartSizeBytes int64  `json:"part_size_bytes"`
	PartCount     int    `json:"part_count"`
	Status        string `json:"status"`
	Generation    string `json:"generation,omitempty"`
}
