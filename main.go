package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	bucket := envOr("LEGAL_ASSET_BUCKET", "legal-assets-demo")
	client := newStorageClient(defaultInfraiBaseURL, apiKey, &http.Client{Timeout: 15 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.ensureBucket(ctx, bucket); err != nil {
		log.Fatal(err)
	}

	handoff := &matterHandoff{storage: client, bucket: bucket, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /matters/intake-upload", jsonHandler(handoff.startIntake))
	mux.HandleFunc("POST /matters/signed-delivery", jsonHandler(handoff.signedDelivery))
	addr := envOr("ADDR", ":8080")
	log.Printf("legal asset handoff listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func jsonHandler[I, O any](handle func(context.Context, I) (O, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input I
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		output, err := handle(r.Context(), input)
		if err != nil {
			status := http.StatusBadRequest
			var apiErr *InfraiError
			if errors.As(err, &apiErr) {
				status = apiErr.Status
				if status < 400 || status >= 500 {
					status = http.StatusBadGateway
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, output)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
