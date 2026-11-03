// Package payments is everything the shop knows about the payment provider (Qosshy Pay,
// the test bank in planeta_qosshy_microservice). Switching to a real provider means
// rewriting this package; the rest of the shop only uses CreatePayment and verified Events.
package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const minSecretLen = 32

var (
	apiURL        string
	apiKey        string
	webhookSecret []byte

	// A timeout is a must: without it one hanging bank call would hang the buyer's request forever.
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// MustConfigure reads the provider settings and stops the server if they are missing.
func MustConfigure() {
	apiURL = strings.TrimRight(os.Getenv("PAYMENT_API_URL"), "/")
	apiKey = os.Getenv("PAYMENT_API_KEY")
	webhookSecret = []byte(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	if apiURL == "" || len(apiKey) < minSecretLen || len(webhookSecret) < minSecretLen {
		log.Fatalf("PAYMENT_API_URL, PAYMENT_API_KEY and PAYMENT_WEBHOOK_SECRET must be set (secrets: at least %d characters)", minSecretLen)
	}
}

// WebhookSecret is the key the bank signs webhooks with.
func WebhookSecret() []byte { return webhookSecret }

type CreateRequest struct {
	AmountTiyn        int64     `json:"amount_tiyn"`
	Currency          string    `json:"currency"`
	Description       string    `json:"description"`
	MerchantReference string    `json:"merchant_reference"` // our payment ID; comes back in webhooks
	CustomerEmail     string    `json:"customer_email,omitempty"`
	ReturnURL         string    `json:"return_url"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type CreateResponse struct {
	PaymentID  string `json:"payment_id"`
	PaymentURL string `json:"payment_url"`
}

// CreatePayment opens a payment session at the bank. idempotencyKey makes a retry of the
// same call (after a timeout, say) return the same session instead of creating a second one.
func CreatePayment(ctx context.Context, req CreateRequest, idempotencyKey string) (CreateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return CreateResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/payments", bytes.NewReader(body))
	if err != nil {
		return CreateResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return CreateResponse{}, fmt.Errorf("payment provider unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return CreateResponse{}, fmt.Errorf("payment provider: HTTP %d: %s", resp.StatusCode, msg)
	}

	var out CreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.PaymentID == "" || out.PaymentURL == "" {
		return CreateResponse{}, fmt.Errorf("payment provider: unexpected response: %v", err)
	}
	return out, nil
}
