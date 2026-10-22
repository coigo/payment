package infra

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
)

type FakeProcessor struct {
	logger *slog.Logger
}

func (f *FakeProcessor) Charge (ctx context.Context, paymentId string, c domain.Charge) error {
	time.Sleep(5 *time.Millisecond)
	f.logger.Info("FakeProcessor: charge", slog.String("paymentId", paymentId))

	return nil
}

func NewFakeProcessor (logger *slog.Logger) * FakeProcessor {
	return &FakeProcessor{logger: logger}
}

type StripeProcessor struct {
	apiKey string
	client *http.Client
}

func (s StripeProcessor) Charge(ctx context.Context, paymentID string, c domain.Charge) error {
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(c.AmountCents, 10))
	form.Set("confirm", "true")
	form.Set("payment_method", "pm_card_visa")
	form.Set("automatic_payment_methods[enabled]", "true")
	form.Set("automatic_payment_methods[allow_redirects]", "never")
	form.Set("metadata[payment_id]", paymentID)
	form.Set("currency", normalizeCurrency(c.Currency))

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.stripe.com/v1/payment_intents", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer" + s.apiKey)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Idempotency-Key", paymentID)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d:", resp.StatusCode)
	}

	return nil
	
}

func NewStripeProcessor (apiKey string) * StripeProcessor {
	return &StripeProcessor{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func normalizeCurrency(c string) string {
	if c == "" { return "brl"}
	out := make([]byte, 0, len(c))
	for i := 0; i < len(c); i++ {
		ch := c[i]
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		out = append(out, ch)
	}
	return string(out)
}