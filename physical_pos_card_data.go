package lunixi

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Card data never goes to the physical POS API: the card is read on the
// device. These are the gateway's rules, applied before a request is sent:
// card-data field names anywhere in a body or filter set, and card numbers in
// the free-text fields the gateway scans.

// Term lists of the card-data key rule (shared parity fixture: cardDataKeyRule).
var (
	posCardNumberTerms = []string{
		"pan", "pannumber", "cardpan", "cardnumber", "cardno", "cardnum", "cardnr", "ccnumber", "ccnum", "primaryaccountnumber",
	}
	posCardSecretTerms = []string{
		"cvv", "cvv2", "cvc", "cvc2", "cvn", "cvn2", "cav2", "cvvcode", "cvccode", "cardcode", "securitycode",
		"cardverificationcode", "cardverificationvalue", "track1", "track2", "track3", "tracki", "trackii", "trackiii",
		"trackdata", "magstripe", "magneticstripe", "pinblock", "pincode", "pinnumber", "pindata", "cardpin",
		"cardholderpin", "encryptedpin", "expdate", "expmonth", "expyear", "expmmyy", "expirydate", "expirymonth",
		"expiryyear", "expirymmyy", "expirationdate", "expirationmonth", "expirationyear", "expiremonth", "expireyear",
		"cardexpiry", "cardexpiration",
	}
	posCardSuffixTerms = []string{
		"cardnumber", "pannumber", "primaryaccountnumber", "cvv", "cvv2", "cvc", "cvc2", "cvvcode", "securitycode",
		"track1", "track2", "trackdata", "pinblock", "expmonth", "expyear", "expirydate", "expirymonth", "expiryyear",
		"expirationdate", "cardexpiry",
	}
	posWholeKeyTerms = []string{"track", "expiry", "expiration", "pin", "cid", "csc"}
	posMaskTokens    = []string{"masked", "mask"}
)

const (
	posDigitRunKeyPattern = `[0-9](?:[ -]?[0-9]){12,}`
	// posMaxScanNodes bounds the key scan; a larger body is treated as
	// carrying card data (fail-closed).
	posMaxScanNodes = 10000
	// Card-number scan modes. posPanItemText (item names only) treats an
	// unseparated 13 or 14 digit number with a valid GTIN check digit as a
	// barcode.
	posPanStrict   = "STRICT"
	posPanItemText = "ITEM_TEXT"
)

var (
	posDigitRunKey    = regexp.MustCompile(posDigitRunKeyPattern)
	posIdentifierKey  = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	posLowerUpper     = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	posUpperRunUpper  = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	posLetterDigit    = regexp.MustCompile(`([A-Za-z])([0-9])`)
	posDigitLetter    = regexp.MustCompile(`([0-9])([A-Za-z])`)
	posTokenSeparator = regexp.MustCompile(`[^a-z0-9]+`)
	posGTIN           = regexp.MustCompile(`^(?:[0-9]{8}|[0-9]{12,14})$`)
)

// posCardDataKeyTokens splits a field name into lower-case word tokens
// (camelCase, digits and separators split).
func posCardDataKeyTokens(key string) []string {
	value := posLowerUpper.ReplaceAllString(key, "${1} ${2}")
	value = posUpperRunUpper.ReplaceAllString(value, "${1} ${2}")
	value = posLetterDigit.ReplaceAllString(value, "${1} ${2}")
	value = posDigitLetter.ReplaceAllString(value, "${1} ${2}")
	// Full Unicode lower-casing keeps the combining dot of U+0130.
	value = strings.ToLower(strings.ReplaceAll(value, "İ", "i̇"))
	var tokens []string
	for _, token := range posTokenSeparator.Split(value, -1) {
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// posIsCardDataKey reports whether a field name is card data (pan,
// cardNumber, cvv, track2Data, expiryDate …). The value is never read.
func posIsCardDataKey(key string) bool {
	tokens := posCardDataKeyTokens(key)
	if len(tokens) == 0 {
		return false
	}
	if slices.Contains(posWholeKeyTerms, strings.Join(tokens, "")) {
		return true
	}
	masked := false
	for _, token := range tokens {
		if slices.Contains(posMaskTokens, token) {
			masked = true
		}
	}
	for start := range tokens {
		joined := ""
		for end := start; end < len(tokens); end++ {
			joined += tokens[end]
			number := slices.Contains(posCardNumberTerms, joined)
			if (number || slices.Contains(posCardSecretTerms, joined)) && !(masked && number) {
				return true
			}
			for _, term := range posCardSuffixTerms {
				if strings.HasSuffix(joined, term) && !(masked && slices.Contains(posCardNumberTerms, term)) {
					return true
				}
			}
		}
	}
	return false
}

func posChildPath(path, key string) string {
	if !posIdentifierKey.MatchString(key) {
		return path + "[?]"
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

// posFindCardDataField returns the path of the first card-data key in a
// decoded body or filter set (items[1].cvv). A key holding a 13+ digit run
// yields the path of the object holding it (root at the top). Keys of one
// object are checked in sorted order before its children are visited.
func posFindCardDataField(value any, root string) (string, bool) {
	type node struct {
		value any
		path  string
	}
	stack := []node{{value, ""}}
	visited := 0
	for len(stack) > 0 {
		if visited >= posMaxScanNodes {
			return root, true
		}
		visited++
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch typed := current.value.(type) {
		case []any:
			for i := len(typed) - 1; i >= 0; i-- {
				stack = append(stack, node{typed[i], current.path + "[" + strconv.Itoa(i) + "]"})
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				if posDigitRunKey.MatchString(key) {
					if current.path == "" {
						return root, true
					}
					return current.path, true
				}
				if posIsCardDataKey(key) {
					return posChildPath(current.path, key), true
				}
			}
			for i := len(keys) - 1; i >= 0; i-- {
				stack = append(stack, node{typed[keys[i]], posChildPath(current.path, keys[i])})
			}
		}
	}
	return "", false
}

func posLetterOrNumber(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

func posASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// posPanCandidates yields the stand-alone 13–19 digit runs of text, plain or
// single space / dash separated, the way the gateway's candidate pattern
// matches them: not glued to a letter or number on either side (a dash glued
// to one joins the run into an identifier such as ORD-4111… or a UUID), the
// longest run that ends at such a boundary, scanning on after each match.
func posPanCandidates(text string, visit func(candidate string) bool) {
	runes := []rune(text)
	n := len(runes)
	boundaryBefore := func(i int) bool {
		if i > 0 && posLetterOrNumber(runes[i-1]) {
			return false
		}
		return !(i > 1 && runes[i-1] == '-' && posLetterOrNumber(runes[i-2]))
	}
	boundaryAfter := func(i int) bool {
		if i < n && posLetterOrNumber(runes[i]) {
			return false
		}
		return !(i+1 < n && runes[i] == '-' && posLetterOrNumber(runes[i+1]))
	}
	for i := 0; i < n; {
		if !posASCIIDigit(runes[i]) || !boundaryBefore(i) {
			i++
			continue
		}
		// ends[k] is the position after the (k+1)-th digit of the run.
		ends := []int{i + 1}
		for j := i + 1; len(ends) < 19; {
			if j < n && posASCIIDigit(runes[j]) {
				j++
			} else if j+1 < n && (runes[j] == ' ' || runes[j] == '-') && posASCIIDigit(runes[j+1]) {
				j += 2
			} else {
				break
			}
			ends = append(ends, j)
		}
		end := -1
		for k := len(ends) - 1; k >= 12; k-- {
			if boundaryAfter(ends[k]) {
				end = ends[k]
				break
			}
		}
		if end < 0 {
			i++
			continue
		}
		if !visit(string(runes[i:end])) {
			return
		}
		i = end
	}
}

func posLuhnValid(digits string) bool {
	sum := 0
	for i := 0; i < len(digits); i++ {
		digit := int(digits[len(digits)-1-i] - '0')
		if i%2 == 1 {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}
	return sum%10 == 0
}

func posGTINValid(digits string) bool {
	if !posGTIN.MatchString(digits) {
		return false
	}
	sum, weight := 0, 3
	for i := len(digits) - 2; i >= 0; i-- {
		sum += int(digits[i]-'0') * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}
	return (10-sum%10)%10 == int(digits[len(digits)-1]-'0')
}

// posContainsPanLikeValue reports whether text holds a stand-alone,
// Luhn-valid 13–19 digit number (plain, space or dash separated). In
// posPanItemText mode an unseparated 13 or 14 digit number with a valid GTIN
// check digit is a barcode, not a card.
func posContainsPanLikeValue(text, mode string) bool {
	found := false
	posPanCandidates(text, func(candidate string) bool {
		digits := strings.NewReplacer(" ", "", "-", "").Replace(candidate)
		if len(digits) < 13 || len(digits) > 19 {
			return true
		}
		if mode == posPanItemText && digits == candidate && len(digits) <= 14 && posGTINValid(digits) {
			return true
		}
		if posLuhnValid(digits) {
			found = true
			return false
		}
		return true
	})
	return found
}
