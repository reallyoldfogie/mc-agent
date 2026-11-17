package models

// ReservationRecorder lets a build loop report a position it has just
// claimed as part of a structure, per Part 4 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md ("every build, not just the
// one in progress" — reservations must accumulate across every structure
// a long-lived agent instance ever builds, not just the one currently in
// progress). Implementations should treat this as fire-and-forget,
// non-blocking, best-effort: a failure to persist a reservation must
// never surface as an error from the placement that triggered it.
type ReservationRecorder interface {
	RecordReservation(pos V3, structureID string)
}
