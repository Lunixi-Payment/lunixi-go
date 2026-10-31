package main

import (
	"encoding/json"
	"errors"
	"html"
	"strings"
)

// RenderCheckoutHTML returns a merchant-page fragment, not a public HTTP handler.
// The caller must authorize the order, validate browser CSRF and send no-store.
func RenderCheckoutHTML(token, nonce string) (string, error) {
	if len(token) < 1 || len(token) > 16384 || strings.IndexFunc(token, func(c rune) bool { return c < 0x21 || c > 0x7e }) >= 0 {
		return "", errors.New("invalid checkout token")
	}
	if nonce != "" && (len(nonce) < 16 || len(nonce) > 128 || strings.IndexFunc(nonce, func(c rune) bool {
		return !asciiLetter(c) && !(c >= '0' && c <= '9') && !strings.ContainsRune("+/_=-", c)
	}) >= 0) {
		return "", errors.New("invalid server CSP nonce")
	}
	config, err := json.Marshal(map[string]any{"type": "payment", "target": "lunixi-checkout-form", "options": map[string]string{"token": token}})
	if err != nil {
		return "", errors.New("checkout configuration unavailable")
	}
	attribute := ""
	if nonce != "" {
		attribute = ` nonce="` + html.EscapeString(nonce) + `"`
	}
	// json.Marshal escapes HTML delimiters, including the script-end sequence.
	return `<div id="lunixi-checkout-form"></div><script src="https://cdn.lunixi.com/lunixi-checkout.min.js"` + attribute + `></script>` +
		`<script type="application/json" id="lunixi-checkout-config"` + attribute + `>` + string(config) + `</script><script` + attribute + `>` +
		`(()=>{const config=JSON.parse(document.getElementById('lunixi-checkout-config').textContent);const checkout=Lunixi.checkout.create(config);checkout.mount('#lunixi-checkout-form');})();</script>`, nil
}

// HostedCheckoutURL validates only the backend value; its token may differ from
// the embedded token. Missing URLs require reconciliation, never a new intent.
func HostedCheckoutURL(value string) (string, error) {
	const prefix = "https://pay.lunixi.com/payment/"
	if !strings.HasPrefix(value, prefix) {
		return "", errors.New("backend paymentPageUrl unavailable or invalid")
	}
	token := strings.TrimPrefix(value, prefix)
	if len(token) < 16 || len(token) > 4096 || strings.IndexFunc(token, func(c rune) bool {
		return !asciiLetter(c) && !(c >= '0' && c <= '9') && !strings.ContainsRune("_.~-", c)
	}) >= 0 {
		return "", errors.New("backend paymentPageUrl unavailable or invalid")
	}
	return value, nil
}

func asciiLetter(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
