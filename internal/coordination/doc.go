// Package coordination owns rebuildable cross-instance rate buckets and
// concurrency leases. Durable request acceptance and usage facts remain in
// PostgreSQL; a Valkey failure therefore denies admission instead of bypassing
// limits.
package coordination
