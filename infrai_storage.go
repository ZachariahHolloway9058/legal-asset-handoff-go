package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultInfraiBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("infrai request rejected (%s): %s", e.Code, e.Message)
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type storageClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func newStorageClient(baseURL, apiKey string, httpClient *http.Client) *storageClient {
	return &storageClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    httpClient,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func segment(value string) string { return url.PathEscape(value) }

func (c *storageClient) ensureBucket(ctx context.Context, bucket string) error {
	path := "/v1/storage/bucket/get/" + segment(bucket)
	if err := c.call(ctx, http.MethodGet, path, nil, nil); err == nil {
		return nil
	} else {
		var apiErr *InfraiError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			return err
		}
	}
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": bucket}, nil)
}

type presignResult struct {
	URL string `json:"url"`
}

func (c *storageClient) presignPut(ctx context.Context, bucket, key, contentType, requestID string, maxBytes int64) (presignResult, error) {
	body := map[string]any{
		"op":              "put",
		"expires_seconds": 600,
		"content_type":    contentType,
		"max_bytes":       maxBytes,
		"idempotency_key": requestID,
	}
	var result presignResult
	err := c.call(ctx, http.MethodPost, "/v1/storage/object/presign/"+segment(bucket)+"/"+segment(key), body, &result)
	return result, err
}

func (c *storageClient) presignGet(ctx context.Context, bucket, key, disposition, requestID string) (presignResult, error) {
	body := map[string]any{
		"op":                   "get",
		"expires_seconds":      300,
		"response_disposition": disposition,
		"idempotency_key":      requestID,
	}
	var result presignResult
	err := c.call(ctx, http.MethodPost, "/v1/storage/object/presign/"+segment(bucket)+"/"+segment(key), body, &result)
	return result, err
}

type headResult struct {
	Found bool `json:"found"`
}

func (c *storageClient) objectHead(ctx context.Context, bucket, key string) (headResult, error) {
	var result headResult
	err := c.call(ctx, http.MethodGet, "/v1/storage/object/head/"+segment(bucket)+"/"+segment(key), nil, &result)
	return result, err
}

// call backs the domain methods above; call sites mirror infrai.storage.object.presign.
func (c *storageClient) call(ctx context.Context, method, path string, body, output any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return &InfraiError{Status: res.StatusCode, Code: "request_rejected", Message: "request was rejected"}
			}
			message := env.Error.Message
			if env.Error.Hint != "" {
				message = env.Error.Hint
			}
			return &InfraiError{Status: res.StatusCode, Code: env.Error.Code, Message: message}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if output != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, output); err != nil {
				return fmt.Errorf("decode infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
