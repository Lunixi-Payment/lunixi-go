package lunixi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// Synthetic order details of the tests.
func testBuyer() Buyer {
	return Buyer{
		Name: "Ada", Surname: "Yilmaz", IdentityNumber: "11111111111", Email: "ada@example.com",
		GSMNumber: "+905000000000", City: "Istanbul", Country: "Turkey", ZipCode: "34732", IP: "203.0.113.10",
	}
}

func testAddress() Address {
	return Address{ContactName: "Ada Yilmaz", Address: "Example Street 1", City: "Istanbul", Country: "Turkey", ZipCode: "34662"}
}

func testBasketItem(price string) BasketItem {
	return BasketItem{ID: "line-1", Name: "Example service", Category1: "Services", ItemType: "VIRTUAL", Price: price}
}

// withOrder adds complete order details with one VIRTUAL line of total to a
// raw body.
func withOrder(body map[string]any, total string) map[string]any {
	body["buyer"] = testBuyer()
	body["billingAddress"] = testAddress()
	body["basketItems"] = []BasketItem{testBasketItem(total)}
	return body
}

func withTestOrder(r *DirectMethodPaymentRequest, total string) *DirectMethodPaymentRequest {
	return r.WithBuyer(testBuyer()).WithBillingAddress(testAddress()).WithBasketItems([]BasketItem{testBasketItem(total)})
}

func TestOrderDetailsValues(t *testing.T) {
	if OrderFieldErrorCodeFieldRequired != "payment.order.field_required" || OrderFieldErrorCodeFieldInvalid != "payment.order.field_invalid" {
		t.Fatal("order field error codes")
	}
	reasons := []string{OrderFieldReasonRequired, OrderFieldReasonRequiredWhenPhysical, OrderFieldReasonInvalidType, OrderFieldReasonInvalidFormat, OrderFieldReasonTooLong, OrderFieldReasonTooFewItems}
	if !slices.Equal(reasons, []string{"required", "required_when_physical", "invalid_type", "invalid_format", "too_long", "too_few_items"}) {
		t.Fatalf("reasons %v", reasons)
	}
	if SubscriptionCustomerErrorCodeFieldRequired != "subscription.customer.field_required" || SubscriptionCustomerErrorCodeFieldInvalid != "subscription.customer.field_invalid" {
		t.Fatal("subscription customer error codes")
	}
	lists := map[string][2][]string{
		"order":    {OrderDetailsRequiredFields(), {"buyer", "billingAddress", "basketItems"}},
		"buyer":    {BuyerRequiredFields(), {"name", "surname", "identityNumber", "email", "gsmNumber", "city", "country", "zipCode", "ip"}},
		"checkout": {CheckoutIntentBuyerOptionalFields(), {"ip"}},
		"address":  {AddressRequiredFields(), {"contactName", "address", "city", "country", "zipCode"}},
		"basket":   {BasketItemRequiredFields(), {"id", "name", "category1", "itemType", "price"}},
		"split":    {SplitPaymentFields(), {"totalAmount", "currency", "ownerRef", "legs", "orderId", "ownerScope", "callbackUrl", "buyer", "billingAddress", "shippingAddress", "basketItems"}},
	}
	for name, pair := range lists {
		if !slices.Equal(pair[0], pair[1]) {
			t.Fatalf("%s: %v", name, pair[0])
		}
	}
	if !slices.Equal(buyerFieldRequirements, []string{"REQUIRED"}) {
		t.Fatalf("buyer field requirements %v", buyerFieldRequirements)
	}
}

func TestMissingOrderDetailsPaths(t *testing.T) {
	missing := func(body any, options OrderDetailsOptions) []string {
		t.Helper()
		paths, err := MissingOrderDetails(body, options)
		if err != nil {
			t.Fatal(err)
		}
		return paths
	}
	if got := missing(map[string]any{}, OrderDetailsOptions{}); !slices.Equal(got, []string{"buyer", "billingAddress", "basketItems"}) {
		t.Fatalf("empty body %v", got)
	}
	if got := missing(withOrder(map[string]any{}, "100"), OrderDetailsOptions{}); len(got) != 0 || got == nil {
		t.Fatalf("complete body %v", got)
	}
	buyer := testBuyer()
	buyer.Email, buyer.IP = " ", ""
	item := testBasketItem("100")
	item.Category1, item.ItemType = "", "PHYSICAL"
	body := map[string]any{"buyer": buyer, "billingAddress": Address{City: "Istanbul"}, "basketItems": []any{item, "line"}}
	want := []string{"buyer.email", "buyer.ip", "billingAddress.contactName", "billingAddress.address", "billingAddress.country", "billingAddress.zipCode", "basketItems.0.category1", "basketItems.1", "shippingAddress"}
	if got := missing(body, OrderDetailsOptions{}); !slices.Equal(got, want) {
		t.Fatalf("partial body %v", got)
	}
	body["shippingAddress"] = Address{ContactName: "Ada Yilmaz"}
	if got := missing(body, OrderDetailsOptions{CheckoutIntent: true}); slices.Contains(got, "buyer.ip") || !slices.Contains(got, "shippingAddress.city") || slices.Contains(got, "shippingAddress") {
		t.Fatalf("checkout intent %v", got)
	}
	if got := missing(map[string]any{"buyer": testBuyer(), "billingAddress": testAddress()}, OrderDetailsOptions{WithoutBasket: true}); len(got) != 0 {
		t.Fatalf("without basket %v", got)
	}
	virtual := withOrder(map[string]any{}, "100")
	virtual["shippingAddress"] = Address{}
	if got := missing(virtual, OrderDetailsOptions{}); len(got) != len(addressRequiredFields) || got[0] != "shippingAddress.contactName" {
		t.Fatalf("given shipping address is checked %v", got)
	}
	if _, err := MissingOrderDetails(map[string]any{"buyer": make(chan int)}, OrderDetailsOptions{}); err == nil {
		t.Fatal("unencodable body accepted")
	}
}

func TestOrderDetailsRequiredBeforeNetwork(t *testing.T) {
	client, requests := newRecordingClient(t, nil)
	ctx := context.Background()
	expect := func(name string, err error, paths ...string) {
		t.Helper()
		if !errors.Is(err, ErrConfiguration) {
			t.Fatalf("%s: %v", name, err)
		}
		for _, path := range paths {
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("%s: %q does not name %s", name, err.Error(), path)
			}
		}
		if strings.Contains(err.Error(), "ada@example.com") {
			t.Fatalf("%s: error carries a value", name)
		}
	}
	intent := validIntent()
	intent.Buyer = nil
	_, err := client.Payments.CreateIntent(ctx, intent, testKey)
	expect("intent without buyer", err, "A payment intent", "buyer")
	intent = validIntent()
	intent.ShippingAddress = nil
	_, err = client.Payments.CreateIntent(ctx, intent, testKey)
	expect("physical intent without shipping", err, "shippingAddress")
	_, err = client.Do(ctx, Request{Method: "POST", Path: "/api/v1/payments/intents", Body: map[string]any{"amount": "100", "currency": "TRY"}, IdempotencyKey: testKey})
	expect("raw intent", err, "buyer", "billingAddress", "basketItems")
	marketplace := marketplaceIntent()
	marketplace.BasketItems = nil
	_, err = client.Payments.CreateMarketplaceIntent(ctx, marketplace, testKey)
	expect("marketplace intent", err, "A marketplace payment intent", "basketItems")
	direct := NewDirectMethodPaymentRequest("1000", "TRY", "ORDER-1").WithMethodFields(PaymentMethodMobileBilling, "mobile_provider", map[string]any{"gsmNumber": "+905321234567"})
	_, err = client.Payments.DirectMethodPayment(ctx, direct, testKey)
	expect("direct", err, "A direct method payment", "buyer")
	noIP := testBuyer()
	noIP.IP = ""
	_, err = client.Payments.DirectMethodPayment(ctx, withTestOrder(direct, "1000").WithBuyer(noIP), testKey)
	expect("direct without ip", err, "buyer.ip")
	split := map[string]any{"totalAmount": "1000", "currency": "TRY", "ownerRef": "customer-1", "legs": []any{map[string]any{"amount": "1000", "cardToken": "saved-test-token"}}}
	_, err = client.Payments.Split(ctx, split, testKey)
	expect("split", err, "A split payment", "buyer", "basketItems")
	split = withOrder(split, "1000")
	split["surprise"] = true
	_, err = client.Payments.Split(ctx, split, testKey)
	expect("unknown split field", err, "surprise")
	card := storedCardRequest()
	card.BillingAddress = &Address{City: "Istanbul"}
	_, err = client.Payments.StoreCard(ctx, card, testKey)
	expect("store card", err, "Storing a card", "billingAddress.zipCode")
	_, err = client.Wallet.Topups.StartCard(ctx, map[string]any{"creditWalletAccountId": "wa_1", "amount": "1000", "currency": "TRY", "buyer": map[string]any{"name": "Ada"}, "billingAddress": testAddress()}, testKey)
	expect("wallet top-up", err, "A wallet card top-up", "buyer.surname", "buyer.ip")
	if len(requests()) != 0 {
		t.Fatalf("incomplete order details reached the network: %+v", requests())
	}
	_, err = client.Payments.Split(ctx, withOrder(map[string]any{"totalAmount": "1000", "currency": "TRY", "ownerRef": "customer-1", "legs": []any{map[string]any{"amount": "1000", "cardToken": "saved-test-token"}}}, "1000"), testKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Wallet.Topups.StartCard(ctx, map[string]any{"creditWalletAccountId": "wa_1", "amount": "1000", "currency": "TRY", "buyer": testBuyer(), "billingAddress": testAddress()}, testKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests()) != 2 {
		t.Fatalf("complete order details were not sent: %+v", requests())
	}
}
