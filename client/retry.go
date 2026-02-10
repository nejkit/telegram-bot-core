package client

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
			time.Sleep(t.Wait)
			continue
		}

		if resp.StatusCode != 429 {
			return resp, nil
		}

		var tgError tgbotapi.APIResponse

		body, _ := io.ReadAll(resp.Body)

		_ = json.Unmarshal(body, &tgError)

		if tgError.ErrorCode != 429 {
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
