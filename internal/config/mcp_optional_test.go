package config_test

import (
	"encoding/json"
	"github.com/sausheong/hand/internal/config"
	"testing"
)

func TestOptionalMCPConfiguration(t *testing.T) {
	var cfg config.Config
	if err := json.Unmarshal([]byte(`{"mcp_servers":[{"name":"required","command":"server"},{"name":"optional","command":"server","optional":true}]}`), &cfg); err != nil {
		t.Fatal(err)
	}
	servers := cfg.ToServerConfigs()
	if len(servers) != 2 || servers[0].Optional || !servers[1].Optional {
		t.Fatalf("mapping %+v", servers)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip config.Config
	if err = json.Unmarshal(b, &roundtrip); err != nil || !roundtrip.MCPServers[1].Optional {
		t.Fatalf("roundtrip %v", err)
	}
}
