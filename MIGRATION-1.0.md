# 1.0.0 migration

All public monetary request and response fields now use canonical base-10 **strings in minor units**. For example, `amount: "1990"` represents TRY 19.90. This includes basket `price`, split and note amounts, capture/refund, balances and fixed fees on supported endpoints. Numeric JSON money is rejected; convert trusted native exact integers explicitly at your application boundary. Rates, basis points, quantities, counts and timestamps remain numeric.

Use the package’s decimal-text conversion helper with the actual currency. JPY has exponent 0, TRY/USD 2, KWD 3. Do not parse decimal text through float/double or assume a factor of 100. `"9007199254740993"` remains exact. Fiat amounts must fit signed 64-bit storage (maximum `"9223372036854775807"`); provider/platform limits may be lower. Crypto is a separate domain.

Canonical strings reject whitespace, signs on nonnegative values, leading zeros, decimals, exponents, `NaN`, `Infinity`, and negative zero. Payments are positive. Settlement totals may be zero; balances may be signed. Only omission retains remaining-amount semantics. Explicit JSON `null` and `"0"` are invalid. Split-refund amount remains required where the existing API requires it.

Update call sites, persisted outbound request schemas and gateway response fixtures together. Preserve an existing attempt’s exact body, idempotency key, environment and credential profile; reconcile historical attempts before creating new ones. The SDK does not silently rewrite historical payment bytes or accept both numeric and string wire money. Native internal exact integer storage may remain unchanged.

Raw webhook verification, signatures, authentication and replay/idempotency protections are unchanged. Upgrade the gateway and integrations as one coordinated breaking cut. Install version 1.0.0 after it is available in this package’s distribution channel.

Go `IntentRequest.Amount`, `BasketItem.Price` and split/refund amounts are strings. Optional capture/refund uses `*string`; nil means remaining. Use `DecimalToMinor(decimal, currency)` for decimal text. The module path remains `github.com/Lunixi-Payment/lunixi-go`.

## Optional capture/refund amounts

The typed `*string` argument retains `nil` as explicit optional absence and omits the JSON property. A non-nil pointer requires a positive canonical string. Raw `Client.Do` capture/refund JSON with an explicit `amount: null` is rejected before transport.

The advanced/raw request boundary also rejects a present `amount: null` on capture/refund routes. Unrelated nullable response fields and merchant metadata keep their existing meaning.
