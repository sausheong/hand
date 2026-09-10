package rpc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
)

// OpenWorkspaceLedger uses a stable workspace identity across session changes
// and client interfaces. The exclusive lock prevents concurrent owners of the
// same request namespace. Callers must close the ledger after dispatcher work.
func OpenWorkspaceLedger(storeDirectory, workspace string) (*Ledger, error) {
	if storeDirectory == "" || workspace == "" {
		return nil, errors.New("ledger store and workspace are required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(absolute))
	return OpenLedger(filepath.Join(storeDirectory, "rpc-"+hex.EncodeToString(digest[:]), "requests.jsonl"))
}
