package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sausheong/hand/internal/config"
)

var routerMetadataCache = struct {
	sync.Mutex
	values map[string]routerMetadataEntry
}{values: make(map[string]routerMetadataEntry)}

type routerMetadataEntry struct {
	metadata config.ProfileMetadata
	expires  time.Time
}

func discoverOpenRouter(ctx context.Context, model string) (config.ProfileMetadata, error) {
	routerMetadataCache.Lock()
	cached, ok := routerMetadataCache.values[model]
	routerMetadataCache.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.metadata, nil
	}
	ctx, cancel := context.WithTimeout(ctx, MetadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return config.ProfileMetadata{}, err
	}
	client := &http.Client{Timeout: MetadataTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return config.ProfileMetadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return config.ProfileMetadata{}, fmt.Errorf("model catalogue HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	if err != nil || len(raw) > 16<<20 {
		return config.ProfileMetadata{}, fmt.Errorf("model catalogue unreadable or oversized")
	}
	metadata, err := decodeRouterMetadata(raw, model)
	if err != nil {
		return metadata, err
	}
	routerMetadataCache.Lock()
	if len(routerMetadataCache.values) >= 128 {
		clear(routerMetadataCache.values)
	}
	routerMetadataCache.values[model] = routerMetadataEntry{metadata, time.Now().Add(15 * time.Minute)}
	routerMetadataCache.Unlock()
	return metadata, nil
}

func decodeRouterMetadata(raw []byte, model string) (config.ProfileMetadata, error) {
	var body struct {
		Data []struct {
			ID      string `json:"id"`
			Context int    `json:"context_length"`
			Top     struct {
				Context int `json:"context_length"`
				Output  int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return config.ProfileMetadata{}, fmt.Errorf("invalid model catalogue")
	}
	for _, entry := range body.Data {
		if entry.ID == model {
			limit := entry.Context
			if entry.Top.Context > 0 && (limit == 0 || entry.Top.Context < limit) {
				limit = entry.Top.Context
			}
			if limit < 2 || entry.Top.Output < 0 {
				return config.ProfileMetadata{}, fmt.Errorf("invalid model limits")
			}
			return config.ProfileMetadata{Verified: true, Source: "openrouter_models:" + model, AdvertisedContext: limit, MaxOutput: entry.Top.Output}, nil
		}
	}
	return config.ProfileMetadata{}, fmt.Errorf("model absent from catalogue")
}
