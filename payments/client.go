// Package payments is everything the shop knows about the payment provider (Qosshy Pay,
// the test bank in planeta_qosshy_microservice). Switching to a real provider means
// rewriting this package; the rest of the shop only uses CreatePayment and verified Events.
package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	status, body, err := post(ctx, "/api/v1/payments", req, idempotencyKey)
	if err != nil {
		return CreateResponse{}, err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return CreateResponse{}, fmt.Errorf("payment provider: HTTP %d: %s", status, body)
	}

	var out CreateResponse
	if err := json.Unmarshal(body, &out); err != nil || out.PaymentID == "" || out.PaymentURL == "" {
		return CreateResponse{}, fmt.Errorf("payment provider: unexpected response: %v", err)
	}
	return out, nil
}

// ErrNotRefundable: the bank says this payment can't be refunded (it never succeeded).
var ErrNotRefundable = errors.New("payment provider: this payment can't be refunded")

type RefundResponse struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"` // "refunded"
}

// Refund asks the bank to give the whole amount of a successful payment back.
// Asking twice is harmless: for an already refunded payment the bank just answers
// "refunded" again and doesn't send the money a second time.
func Refund(ctx context.Context, providerPaymentID string) (RefundResponse, error) {
	path := "/api/v1/payments/" + url.PathEscape(providerPaymentID) + "/refund"
	status, body, err := post(ctx, path, struct{}{}, "refund-"+providerPaymentID)
	if err != nil {
		return RefundResponse{}, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusConflict:
		return RefundResponse{}, ErrNotRefundable
	default:
		return RefundResponse{}, fmt.Errorf("payment provider: HTTP %d: %s", status, body)
	}

	var out RefundResponse
	if err := json.Unmarshal(body, &out); err != nil || out.Status != "refunded" {
		return RefundResponse{}, fmt.Errorf("payment provider: unexpected refund response %q", body)
	}
	return out, nil
}

// post sends one JSON request to the bank and returns the HTTP status and (up to 64 KB of) the body.
func post(ctx context.Context, path string, payload any, idempotencyKey string) (int, []byte, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return 0, nil, fmt.Errorf("payment provider unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return 0, nil, fmt.Errorf("payment provider: reading the answer: %w", err)
	}
	return resp.StatusCode, body, nil
}
