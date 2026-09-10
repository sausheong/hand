//go:build !darwin && !linux

package rpc

var lockLedger = unsupportedLedgerLock
