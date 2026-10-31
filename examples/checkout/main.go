// This example creates a TEST checkout intent from trusted merchant values.
// Run with a persisted order/attempt ID and the matching TEST credentials.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	lunixi "github.com/Lunixi-Payment/lunixi-go"
)

func run() error {
	keyID := os.Getenv("LUNIXI_KEY_ID")
	if !strings.HasPrefix(keyID, "mk_test_") {
		return fmt.Errorf("this example requires a TEST merchant key")
	}
	orderID, attemptKey := os.Getenv("LUNIXI_ORDER_ID"), os.Getenv("LUNIXI_ATTEMPT_KEY")
	if orderID == "" || attemptKey == "" {
		return fmt.Errorf("set a persisted LUNIXI_ORDER_ID and LUNIXI_ATTEMPT_KEY before running")
	}
	client, err := lunixi.New(lunixi.Config{BaseURL: os.Getenv("LUNIXI_BASE_URL"), KeyID: keyID, PrivateKey: os.Getenv("LUNIXI_PRIVATE_KEY_PEM")})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := client.Payments.CreateIntent(ctx, lunixi.IntentRequest{
		Amount: 10050, Currency: "TRY", OrderID: orderID,
		CallbackURL: "https://merchant.example/payment/return",
		BasketItems: []lunixi.BasketItem{{ID: "basket-1", Name: "Example product", ItemType: "PHYSICAL", Price: 10050}},
	}, attemptKey)
	if err != nil {
		return err
	}
	var checkout struct {
		PaymentID      string `json:"paymentId"`
		Token          string `json:"token"`
		PaymentPageURL string `json:"paymentPageUrl"`
	}
	if err := response.DecodeData(&checkout); err != nil || checkout.PaymentID == "" || checkout.Token == "" {
		return fmt.Errorf("gateway response did not contain a checkout token")
	}
	mode := os.Getenv("LUNIXI_CHECKOUT_MODE")
	if mode == "" || mode == "embedded" {
		if _, err := RenderCheckoutHTML(checkout.Token, ""); err != nil {
			return err
		}
	} else if mode == "hosted" {
		if _, err := HostedCheckoutURL(checkout.PaymentPageURL); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("LUNIXI_CHECKOUT_MODE must be embedded or hosted")
	}
	// In a real server endpoint send Token only to this order's authenticated
	// customer with Cache-Control: no-store; initialize the existing Lunixi form.
	// Keep it out of access logs, analytics, URLs and error reports.
	return json.NewEncoder(os.Stdout).Encode(struct {
		CheckoutReady bool `json:"checkoutReady"`
	}{true})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
