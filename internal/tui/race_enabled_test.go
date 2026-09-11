//go:build race

package tui

// Validation runs the wall-clock performance gate separately, without race or
// coverage instrumentation. The instrumented suite still executes the workload.
const raceInstrumented = true
