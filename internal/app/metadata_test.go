package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/config"
)

func TestMetadataUsesActiveContextAndExplicitPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("wrong credential reference")
		}
		fmt.Fprint(w, `{"default_generation_settings":{"n_ctx":24576},"n_ctx_train":262144,"model_path":"model.gguf"}`)
	}))
	defer server.Close()
	t.Setenv("METADATA_FIXTURE_KEY", "fixture-key")
	p := config.ModelProfile{Provider: "openai", Model: "alias", Endpoint: server.URL + "/v1", CredentialEnv: "METADATA_FIXTURE_KEY", MetadataProtocol: "llamacpp_props", MetadataURL: server.URL + "/props"}
	metadata, err := DiscoverProfile(context.Background(), p)
	if err != nil || metadata.ActiveContext != 24576 || metadata.AdvertisedContext != 0 || !metadata.Verified || !strings.Contains(metadata.Source, "#sha256=") {
		t.Fatal(metadata, err)
	}
	prepared, err := PrepareProfile(context.Background(), p)
	if err != nil || prepared.ContextLimit != 24576 {
		t.Fatal(prepared, err)
	}
	p.ContextLimit = 16384
	prepared, err = PrepareProfile(context.Background(), p)
	if err != nil || prepared.ContextLimit != 16384 {
		t.Fatal("explicit override lost", err)
	}
}

func TestMetadataRejectsRedirectOversizeAndInvalidProperties(t *testing.T) {
	for _, name := range []string{"redirect", "oversize", "invalid", "router", "advertised-only", "error"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch name {
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1:1/secret", http.StatusFound)
				case "oversize":
					fmt.Fprint(w, strings.Repeat(" ", MetadataMaxBytes+1))
				case "invalid":
					fmt.Fprint(w, "not JSON")
				case "router":
					fmt.Fprint(w, `{"role":"router","default_generation_settings":{"n_ctx":32768}}`)
				case "advertised-only":
					fmt.Fprint(w, `{"n_ctx_train":131072}`)
				case "error":
					http.Error(w, "secret server detail", http.StatusUnauthorized)
				}
			}))
			defer server.Close()
			p := config.ModelProfile{Provider: "local", Model: "alias", Endpoint: server.URL + "/v1", MetadataProtocol: "llamacpp_props", MetadataURL: server.URL + "/props"}
			if m, err := DiscoverProfile(context.Background(), p); err == nil || m.Verified || strings.Contains(err.Error(), "secret server detail") {
				t.Fatal(m, err)
			}
		})
	}
}

func TestMetadataCancellationAndOriginBoundary(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	p := config.ModelProfile{Provider: "local", Model: "alias", Endpoint: server.URL + "/v1", MetadataProtocol: "llamacpp_props", MetadataURL: server.URL + "/props"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := DiscoverProfile(ctx, p); result <- err }()
	<-started
	cancel()
	if <-result == nil {
		t.Fatal("cancelled metadata accepted")
	}
	p.MetadataURL = "https://elsewhere.invalid/props"
	if p.Validate() == nil {
		t.Fatal("credential origin crossover accepted")
	}
}
