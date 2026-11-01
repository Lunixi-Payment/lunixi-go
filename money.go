package lunixi

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var positiveMinorPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
var nonnegativeMinorPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var signedMinorPattern = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)

func validMinor(value string, positive, signed bool) bool {
	pattern := nonnegativeMinorPattern
	if positive {
		pattern = positiveMinorPattern
	}
	if signed {
		pattern = signedMinorPattern
	}
	if !pattern.MatchString(value) {
		return false
	}
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

// DecimalToMinor converts decimal text with a supported currency exponent, without floating point.
func DecimalToMinor(value, currency string) (string, error) {
	exponent := 2
	switch currency {
	case "JPY":
		exponent = 0
	case "KWD":
		exponent = 3
	case "TRY", "USD", "EUR", "GBP", "RUB", "AED", "SAR", "QAR", "CHF", "CAD", "AUD", "CNY", "AZN":
	default:
		return "", invalid("unsupported currency")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || !nonnegativeMinorPattern.MatchString(parts[0]) {
		return "", invalid("decimal text")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" {
			return "", invalid("decimal text")
		}
		for _, c := range fraction {
			if c < '0' || c > '9' {
				return "", invalid("decimal text")
			}
		}
	}
	if len(fraction) > exponent {
		if strings.Trim(fraction[exponent:], "0") != "" {
			return "", invalid("currency precision")
		}
		fraction = fraction[:exponent]
	}
	value = strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", exponent-len(fraction)), "0")
	if value == "" {
		value = "0"
	}
	if !validMinor(value, false, false) {
		return "", invalid("signed 64-bit fiat amount")
	}
	return value, nil
}
func validateMoneyJSON(raw []byte, response bool) error {
	if len(raw) == 0 {
		return nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return invalid("JSON body")
	}
	return validateMoneyValue(value, response)
}
func validateMoneyValue(value any, response bool) error {
	switch v := value.(type) {
	case []any:
		for _, entry := range v {
			if err := validateMoneyValue(entry, response); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, entry := range v {
			if entry == nil || key == "metadata" || key == "fields" || key == "inputFields" || key == "insuranceData" {
				continue
			}
			money := false
			switch key {
			case "amount", "amountMinor", "authorizedAmount", "availableBalance", "balanceMinor", "buyerTotalCharge", "cancelledPremiumAmount", "capturedAmount", "capturedAmountMinor", "chargeAmount", "collectedAmountMinor", "collectedPremiumAmount", "compensatedAmount", "disputedAmount", "feeTotal", "fixedFee", "inForcePremiumAmount", "insuranceValueAmount", "legAmount", "legsAmount", "lineTotalMinor", "maxAmount", "maxAmountMinor", "minAmount", "minAmountMinor", "nonRefundableAmount", "paidAmount", "paidPrice", "pendingBalance", "premiumAmount", "price", "refundAmount", "refundOpenPremiumAmount", "refundableAmount", "refundedAmount", "refundedAmountMinor", "refundedPremiumAmount", "requestedAmount", "reservedBalance", "sourceAmountMinor", "targetAmountMinor", "taxAmountMinor", "taxTotal", "totalAmount", "unitPriceMinor":
				money = true
			}
			if money {
				text, ok := entry.(string)
				if !ok || !validMinor(text, !response && (key == "amount" || key == "price" || key == "totalAmount"), strings.Contains(strings.ToLower(key), "balance")) {
					return invalid("canonical minor-unit string required: " + key)
				}
			} else {
				switch key {
				case "data", "details", "items", "rows", "results", "payments", "operations", "balances", "wallets", "legs", "aggregate", "refund", "refunds", "totals", "charge", "fees", "fee", "balance", "splits", "sellers", "basketItems", "promissoryNote", "schedule", "notes", "lines", "offers", "policies", "reconciliation", "quotes", "payment", "transactions", "accounts", "settlements", "payouts", "limits":
					if err := validateMoneyValue(entry, response); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func fiatRoute(path string) bool {
	for _, base := range []string{"/api/v1/payments", "/api/v1/wallet", "/api/v1/insurance", "/api/v1/bank-connect", "/api/v1/fraud"} {
		if path == base || strings.HasPrefix(path, base+"/") || strings.HasPrefix(path, base+"?") {
			return true
		}
	}
	return false
}
