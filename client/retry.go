package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type apiResponse struct {
	OK         bool `json:"ok"`
	ErrorCode  int  `json:"error_code,omitempty"`
	Parameters struct {
		RetryAfter int `json:"retry_after,omitempty"`
	} `json:"parameters,omitempty"`
}

type RetryTransport struct {
	Base    http.RoundTripper
	Retries int
	Wait    time.Duration
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := 0; i <= t.Retries; i++ {
		resp, err = t.Base.RoundTrip(req)

		if err != nil {
			time.Sleep(t.Wait)
			continue
		}

		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusInternalServerError {
			resp.Body.Close()
			time.Sleep(t.Wait)
			continue
		}

		if resp.StatusCode != 429 {
			return resp, nil
		}

		var tgError apiResponse

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		_ = json.Unmarshal(body, &tgError)

		if tgError.ErrorCode != 429 {
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return resp, err
		}

		retryAfter := tgError.Parameters.RetryAfter

		if retryAfter == 0 {
			time.Sleep(t.Wait)
			continue
		}

		time.Sleep(time.Duration(retryAfter) * time.Second)
	}

	return resp, err
}
