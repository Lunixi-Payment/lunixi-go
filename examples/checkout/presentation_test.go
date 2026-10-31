package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMerchantFormUsesExistingBundleAndEscapedStructuredConfiguration(t *testing.T) {
	token := `</script><svg/onload=unexpected()>`
	page, err := RenderCheckoutHTML(token, "server_nonce_123456789")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, token) || !strings.Contains(page, "https://cdn.lunixi.com/lunixi-checkout.min.js") || !strings.Contains(page, "checkout.mount('#lunixi-checkout-form')") {
		t.Fatal("unsafe or missing official form")
	}
	start := strings.Index(page, `id="lunixi-checkout-config"`)
	data := page[start:]
	data = data[strings.Index(data, ">")+1 : strings.Index(data, "</script>")]
	var config struct {
		Type    string
		Target  string
		Options struct{ Token string }
	}
	if json.Unmarshal([]byte(data), &config) != nil || config.Type != "payment" || config.Target != "lunixi-checkout-form" || config.Options.Token != token {
		t.Fatal("incorrect checkout object")
	}
	if _, err := RenderCheckoutHTML("token", `bad"nonce`); err == nil {
		t.Fatal("unsafe CSP nonce accepted")
	}
}

func TestHostedChoiceUsesIndependentBackendURLAndRejectsUnsafeTargets(t *testing.T) {
	want := "https://pay.lunixi.com/payment/independent-backend-token"
	if got, err := HostedCheckoutURL(want); err != nil || got != want {
		t.Fatal("valid independent backend URL rejected")
	}
	for _, value := range []string{"", "http://pay.lunixi.com/payment/independent-backend-token", "https://evil.example/payment/independent-backend-token", "https://pay.lunixi.com:443/payment/independent-backend-token", "https://user@pay.lunixi.com/payment/independent-backend-token", want + "?x=1", want + "#x", want + "\n", "https://pay.lunixi.com/payment/short", "https://pay.lunixi.com/payment/encoded%2Ftoken-123456"} {
		if _, err := HostedCheckoutURL(value); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
}
