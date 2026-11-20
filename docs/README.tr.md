# Lunixi Go SDK

**1.0.0**, kanonik küçük birim string para sözleşmesini kullanır. [Geçiş rehberini](../MIGRATION-1.0.md) uygulayın; yayımlanan [v2.0.0 modülünü](https://github.com/Lunixi-Payment/lunixi-go/tree/v2.0.0) (sürüm 2 ödeme uçlarında sipariş bilgilerini ister, import yolu `/v2` ile biter) aşağıdaki komutla yükleyin. `go.sum` dosyasını saklayın ve modülü Go proxy/checksum veritabanıyla doğrulayın.

1.0.0 sürümü mağaza sitesi için somut bir sunum örneği ekler. **Varsayılan gömülü ödemedir**: yalnız resmi Lunixi bundle yüklenir ve mevcut form mount edilir; yöntem ve çoklu kart seçenekleri Lunixi panelinde kalır. İsteğe bağlı yönlendirme yalnız backend `paymentPageUrl` adresini, tam HTTPS `pay.lunixi.com/payment/` doğrulamasından sonra kullanır. Adresin token’ı gömülü token’dan farklı olabilir. Eksik adres gömülü ödemeyi engellemez; yönlendirme aynı deneme/anahtarla inceleme veya uzlaştırmada kalır. Tarayıcı token’ından adres üretmeyin. Formdan önce müşteri/sipariş sahipliğini ve yerel tarayıcı CSRF’yi doğrulayın; `Cache-Control: private, no-store` gönderin, mağazanın CSP nonce değerini koruyun; token, HTML ve yönlendirme adreslerini loglamayın. Genel yanıt verisi `paymentPageUrl` alanını korur (artık her ürünün tipli istemcisi vardır).

[English README](../README.md) · [Go dokümantasyonu](https://docs.lunixi.com/tr/sdks/go)

Go 1.26+ sunucu uygulamaları için Lunixi istemcisidir. Çalışma zamanı bağımlılığı yoktur; `net/http`, Gin, Echo ve Fiber ile kullanılabilir.

```sh
go get github.com/Lunixi-Payment/lunixi-go/v2@v2.0.0
```

```go
client, err := lunixi.New(lunixi.Config{
    BaseURL: "https://api-gateway.lunixi.com",
    KeyID: os.Getenv("LUNIXI_KEY_ID"),
    PrivateKey: os.Getenv("LUNIXI_PRIVATE_KEY_PEM"),
    MaxRetries: 2,
})
if err != nil { return err }

response, err := client.Payments.CreateIntent(ctx, lunixi.IntentRequest{
    Amount: "10050", Currency: "TRY", OrderID: "ORD-1001",
    CallbackURL: "https://merchant.example/payment/return",
    // Alıcı, fatura adresi ve sepet zorunludur; PHYSICAL kalemde teslimat adresi de
    // gerekir. Değerleri siparişinizden alın, yer tutucu kullanmayın. Intent'te
    // alıcı IP'si isteğe bağlıdır: Lunixi formu IP'yi tarayıcıdan alır.
    Buyer: &lunixi.Buyer{
        Name: "Ada", Surname: "Yılmaz", IdentityNumber: "11111111111",
        Email: "ada@example.com", GSMNumber: "+905000000000",
        City: "İstanbul", Country: "Türkiye", ZipCode: "34732",
    },
    BillingAddress: &lunixi.Address{
        ContactName: "Ada Yılmaz", Address: "Örnek Sok. No: 1",
        City: "İstanbul", Country: "Türkiye", ZipCode: "34662",
    },
    BasketItems: []lunixi.BasketItem{
        {ID: "line-1", Name: "Danışmanlık hizmeti", Category1: "Hizmet", ItemType: "VIRTUAL", Price: "10050"},
    },
}, kayitliDenemeAnahtari)
```

`lunixi` import yolu `github.com/Lunixi-Payment/lunixi-go/v2` şeklindedir. İstemciyi her merchant anahtarı için bir kez oluşturup paylaşın. SDK Ed25519 imzasını, token önbelleğini ve eşzamanlı yenilemeyi yönetir. Merchant istekleri GET dahil imzalıdır. Abonelik, KYC, kimlik ve pazaryeri bayi rotaları, gateway bunları böyle doğruladığı için yalnız erişim token'ıyla gönderilir; pazaryeri bölünmüş tahsilatları imzalı kalır. Özel anahtar sunucuda kalır; PKCS8 PEM / base64 DER / base64 Ed25519 seed kabul edilir. TEST/LIVE ortamını merchant anahtarı belirler. BaseURL güvenilir HTTPS kökeni olmalıdır; yönlendirmeler izlenmez.

`Amount` ve sepet `Price` alanları kanonik alt birim string değeridir: `"10050"` = 100,50 TRY. Sepet toplamı `Amount` ile aynı olmalıdır. Kaldırılmış kök alanlar `price`, `paidPrice`, `autoCapture` gönderilmez. Taksit ve vade farkını Lunixi formu yönetir; `EnabledInstallments` bu ödeme için formun sunacağı taksit sayılarını yalnızca daraltır (ör. en fazla 3 taksit için `[]int{1, 2, 3}`, yalnız tek çekim için `[]int{1}`). Form, panel ayarlarınızın, kartın ve platform üst sınırının da izin verdiği sayıları gösterir; nil kısıt yok demektir. Sayılar 1–36 arası, tekrarsız olmalı ve 1'i içermelidir; aksi hâlde `CreateIntent` istek göndermeden `ErrConfiguration` döner. Liste artan sırada gönderilir. `BasketItem.ProductCategory` (`productCategory`) isteğe bağlı Lunixi ürün kategorisidir; yasal taksit sınırı olan ödeme yöntemleri (ör. alışveriş kredisi) ve diğer platform özellikleri tarafından kullanılır. Değer için `ProductCategory*` sabitlerini kullanın (`ProductCategories()` bu sürümün bildiği kodları listeler). Boş bırakılırsa alan gönderilmez ve mevcut davranış korunur; bilinmeyen kod API tarafından HTTP 400 ile reddedilir. Sözlük yalnız sona ekleme ile büyür; alan bu yüzden düz string'dir.

Her ödeme gövdesi sipariş bilgilerini taşır: `Buyer` (`Name`, `Surname`, `IdentityNumber` — T.C. kimlik ya da pasaport numarası, `Email` — ASCII, `GSMNumber`, `City`, `Country`, `ZipCode`, `IP` — alıcının tarayıcı IP'si), `BillingAddress` (`ContactName`, `Address`, `City`, `Country`, `ZipCode`) ve en az bir `BasketItems` kalemi (`ID`, `Name`, `Category1`, `ItemType`, `Price`; fiyatların toplamı tutara eşittir); `PHYSICAL` kalemde `ShippingAddress` de gerekir. `CreateIntent` ve pazaryeri intent'lerinde `IP` gönderilmeyebilir: Lunixi formu IP'yi alıcının tarayıcısından alır. `CreateIntent`, `CreateMarketplaceIntent`, `Marketplace.CreateCheckoutIntent`, `DirectMethodPayment`, `Split`, pazaryeri kart çekimleri, `StoreCard` (yalnız alıcı ve fatura adresi) ve `Wallet.Topups.StartCard` (alıcı ve fatura adresi, teslimat adresi isteğe bağlı) eksik alanların yollarını (`buyer.email`, `basketItems.0.category1`, `shippingAddress`) sayan `ErrConfiguration` ile göndermeden durur; bu rotalara giden ham `Client.Do` gövdeleri de aynı şekilde denetlenir. `MissingOrderDetails(body, OrderDetailsOptions{})` aynı listeyi döner; alan listeleri `OrderDetailsRequiredFields()`, `BuyerRequiredFields()`, `CheckoutIntentBuyerOptionalFields()`, `AddressRequiredFields()` ve `BasketItemRequiredFields()` ile okunur. Gateway eksik alana 400 `OrderFieldErrorCodeFieldRequired` (`payment.order.field_required`), geçersiz değere `OrderFieldErrorCodeFieldInvalid` (`payment.order.field_invalid`) döner; `details.reason` bir `OrderFieldReason*` değeridir.

Token yalnız ilgili siparişin yetkili müşterisine `Cache-Control: no-store` ile verilir; loglanmaz. PAN/CVV bu örneklere girmez.

## Kapsam

- `Payments`: `CreateIntent`, `Get`, `List`, `Methods`, `ListPaymentMethods`, `Capture`, `Refund`, `Void`.
- `Payments` ödeme yöntemleri: `DirectMethodPayment`, `ConfirmPaymentAction`, `ResendPaymentAction`.
- `Payments` senet: `GetPromissoryNotes`, `CancelPromissoryNotes`, `DownloadPromissoryNoteDocument`.
- `Payments` Para Transferi: `ListBankTransferExceptions`, `AcceptBankTransferException`, `MarkBankTransferExceptionRefunded`.
- `Payments` parçalı ödeme: `Split`, `GetSplit`, `ListSplits`, `RefundSplit`.
- `Payments` kartlar: `BinInfo`, `InstallmentOptions`, `CardTaxonomy`, `StoreCard`, `ListStoredCards`, `GetStoredCard`, `DeactivateStoredCard`.
- `Payments` pazaryeri ve raporlama: `CreateMarketplaceIntent`, `FailoverRecoveryAnalytics`, `ProviderCredentialAnalytics`.
- `PaymentLinks`: `Create`, `List`, `Get`, `Update`, `Pause`, `Resume`, `Deactivate`, `Action`, `Send`, `QR`, `ListAttempts`, `ListPayments`, `CreateImageUpload`, `ConfirmImageUpload`, `UploadImage`.
- `Reference`: `ListProvinces`, `ListDistricts`, `ListNeighborhoods`.
- `Fraud`: `Evaluate`, `RecordEvent`, `RecordBatch`, `Decisions`, `Decision`, `LatestForPayment`.
- `PhysicalPos`, `PhysicalPosManagement`, `Wallet`, `BankConnect`, `Sanction`, `Insurance`, `Subscriptions`, `Kyc`, `Identity`, `Marketplace`: aşağıdaki bölümlerde anlatılır.
- `Client.Do`: diğer belgelenmiş `/api/v1/` uçlarına aynı imzalı taşıma katmanıyla erişim. Her ürünün tipli istemcisi vardır; `Do` yalnız bunların dışındaki uçlar içindir.
- `WebhookVerifier` ve `SQLInbox`: ham gövde v2 imzası ve kalıcı transaction içinde tekrar engelleme.

Yanıtta `Data`, `PageInfo`, asıl zarf ve ikili `Bytes` korunur. `DecodeData` arayüz değerleri için `json.Number` kullanır. `Split` belgelenmiş gateway map'ini (`SplitPaymentFields()`: tutarlar, parçalar ve sipariş bilgileri `buyer`, `billingAddress`, `shippingAddress`, `basketItems`) alır ve güvenli toplam/parça tutarlarını, 1–5 parçayı, parça toplamını ve sipariş bilgilerini yerelde denetler. Kart parçası (`paymentMethod` boş veya `CARD`) yöntem bloğu taşımaz; kart dışı parça, `providerCode` dahil yalnız kendi bloğunu (`SplitLegBlockFields()`) taşıyan ve kart alanı içermeyen bir `MOBILE_BILLING`, `WALLET` veya `PROMISSORY_NOTE` yöntemidir; senet parçası vade tablosu değil `noteCount` verir. `MEAL_CARD`, `OPEN_BANKING`, `SHOPPING_LOAN` ve `BANK_TRANSFER` parça olamaz. Yönteme özgü diğer alanları API şeması belirler. `ListSplits`, `SplitListFilters` alır (durum ve para birimi listeleri, oluşturma aralığı, sipariş, sahip, 1–100 arası limit, `After` imleci). Parçalı ödeme beklerken HTTP 400 içeren `Response` dönebilir; bu tahsilat değildir. Siparişi yalnız imzalı webhook / yetkili sunucu sorgusunda sipariş, merchant, ödeme kimliği, tutar, para birimi ve nihai tahsilat durumunu eşleştirerek tamamlayın. Tarayıcı dönüşü, HTTP 200 veya `AUTHORIZED` ödeme kanıtı değildir.

## Kartlar, taksitler ve pazaryeri intent'leri

```go
quote, err := client.Payments.InstallmentOptions(ctx, lunixi.InstallmentOptionsRequest{
    Amount: "120000", Currency: "TRY", BinOrPan: bin, Installment: 3,
    EnabledInstallments: []int{1, 2, 3},
})
stored, err := client.Payments.StoreCard(ctx, lunixi.StoreCardRequest{
    Card: cardFields, CardUserKey: buyerKey, CallbackURL: "https://merchant.example/cards/return",
    Buyer: &buyer, BillingAddress: &billingAddress, // zorunlu; sepet yok
}, persistedStoreKey)
```

`BinInfo` 6 veya 8 haneli bir BIN ya da kart numarası öneki alır (yalnız rakam); SDK kart numaralarını veya güvenlik kodlarını hiçbir zaman hata mesajına koymaz, siz de kart alanlarını loglamayın. `InstallmentOptions`, `Amount` (`Currency` ile) veya `IntentID` ister; `EnabledInstallments`, `IntentRequest` kuralını izler ve artan sırada gönderilir. `Installment` 1'den büyük ve BIN verilmişse yanıt, `DirectMethodPaymentRequest.WithInstallment` için `quoteToken` değerini taşır. `StoreCard` ve `DeactivateStoredCard` idempotency anahtarı ister; `ListStoredCards(cardUserKey)` ve `GetStoredCard(token)` saklı kartları okur.

`CreateMarketplaceIntent` bir `MarketplaceIntentRequest` alır: `IntentRequest` alanlarına ek olarak `Sellers` (`SubDealerID`, pozitif küçük birim metin olarak `AmountMinor`, isteğe bağlı `Scenario` nesnesi) ve `MultiSellerPolicy`. Satıcı tutarlarının toplamı `Amount` değerine eşit olmalıdır; başka bir toplam göndermeden önce reddedilir. Anahtar isteğe bağlıdır; yeniden denemelerin güvenli olması için her deneme için sabit bir anahtar verin. `FailoverRecoveryAnalytics` ve `ProviderCredentialAnalytics(credentialID, filters)` merchant raporlarını okur.

## Ödeme linkleri

```go
link, err := client.PaymentLinks.Create(ctx, lunixi.PaymentLinkRequest{
    Usage: lunixi.PaymentLinkUsageSingleUse, AmountMode: lunixi.PaymentLinkAmountModeFixed,
    Currency: "TRY", AmountMinor: "150000", Title: "Invoice 42",
    Recipient: &lunixi.PaymentLinkRecipient{Email: "buyer@example.com"},
}, persistedLinkKey)
image, err := client.PaymentLinks.UploadImage(ctx, "logo.png", lunixi.PaymentLinkImageContentTypePNG, pngBytes)
```

Okumalar dahil her ödeme linki çağrısı imzalıdır; ortamı imzalayan anahtar belirler, bu yüzden TEST anahtarı yalnız TEST linklerini görür. `Create` ve `Send` bir idempotency anahtarı ister (8–200 yazdırılabilir ASCII karakter); `Update`, `Pause`, `Resume` ve `Deactivate` isteğe bağlı anahtar alır. Kullanım şekli, tutar modu ve para birimi oluşturmada sabitlenir: `Update(ctx, linkID, expectedRowVersion, changes, key)` yalnız `PaymentLinkSpecFields()` adlarını alır, `PaymentLinkImmutableFields()` alanlarını reddeder, `nil` verilen alanı temizler ve `time.Time` değerini RFC 3339 UTC olarak gönderir. Değer kümeleri `PaymentLink*`, `BuyerFieldRequirementRequired` (alıcı alanının tek değeri: link sayfası beş alıcı alanını her zaman sorar) ve `CustomFieldType*` sabitleridir; bilinmeyen değerler göndermeden önce reddedilir, iş kuralları API'ye bırakılır (422 `payment_link.validation.invalid`). `List`, `ListAttempts` ve `ListPayments`, `PageInfo` ile `Data.items` döndürür; `pageInfo.nextCursor` değerini `Cursor` olarak verin. `QR`, SVG veya PNG görseli `Response.Bytes` içinde döndürür.

Link görseli üç adımda yüklenir: `CreateImageUpload` yükleme hedefini döndürür, dosya tam olarak `UploadHeaders` başlıklarıyla `UploadURL` adresine PUT edilir ve `ConfirmImageUpload` görseli yayımlar (PNG, JPEG veya WebP, en fazla 2 MiB). `UploadImage` üçünü birden çalıştırır. PUT isteği yapılandırdığınız HTTP istemcisiyle depolama sunucusuna gider ama imzalı taşıma katmanından geçmez: oraya erişim token'ı, imza, digest veya idempotency başlığı gönderilmez. Başarısız PUT, depolamanın durum ve koduyla bir `APIError` döndürür (ör. adresin süresi dolduktan sonra `AccessDenied`) ve yeniden denenmez; `UploadImage` çağrısını tekrar yapın.

## Sunucudan ödeme yöntemleri

`ListPaymentMethods`, hesabınıza açık yöntemleri her yöntemin `blockName`, `providerSelection`, `providerSelectionMode` değerleri ve sağlayıcıların `inputFields` alanlarıyla listeler; tek yöntem için bir kod (`lunixi.PaymentMethodMobileBilling`), tümü için `""` verin. Sağlayıcı kodlarını her zaman bu yanıttan okuyun.

```go
request := lunixi.NewDirectMethodPaymentRequest("10000", "TRY", "ORD-1001").
    WithMethodFields(lunixi.PaymentMethodMobileBilling, providerCode, map[string]any{
        "gsmNumber": "+905321234567",
    }).
    WithBuyer(buyer). // IP dahil: alıcının tarayıcı IP'si
    WithBillingAddress(billingAddress).
    WithBasketItems(basketItems)
response, err := client.Payments.DirectMethodPayment(ctx, request, persistedAttemptKey)
if err != nil { return err }
// HTTP 202 with data.action when the buyer must act (PaymentActionType*).
accepted := true
_, err = client.Payments.ConfirmPaymentAction(ctx, paymentID, actionID, lunixi.PaymentActionConfirmRequest{
    OTP: "123456", ConsentAccepted: &accepted,
})
```

`DirectMethodPayment` idempotency anahtarı ister. Göndermeden önce şunları denetler (`/api/v1/payments/direct/payment` adresine giden ham `Client.Do` gövdeleri için de): yalnız bilinen alanlar; `paymentMethod`, `amount`, `currency`, `orderId`; kart alanları (`CardOnlyFields()`) yalnız `CARD` ile; tam olarak seçilen yöntemin bloğu (`BlockNameOf`, `MethodBlockFields()`); değerleri metin (en fazla 1024 karakter), sayı, boolean, `ADDRESS` nesnesi veya en fazla 10 nesnelik `GROUP` listesi olan, en fazla 32 alanlı blok; `SHOPPING_LOAN` için 1–100 sepet kalemi. Bloğu olmayan bir yöntem henüz bu yolla ödenemez. Kartlar `WithCard` kullanır (3D Secure için `WithForce3D(true)`). `ConfirmPaymentAction` yalnız `otp` (rakamlar), `consentAccepted` ve `fields` gönderir; `ResendPaymentAction` yeni kod ister ve önceki adımı kapatır.

### Senetle Ödeme

Planı ya vade tablosu ya da senet adedi olarak verin, ikisini birden değil:

```go
request := lunixi.NewDirectMethodPaymentRequest("120000", "TRY", "ORD-1002").
    WithMethodFields(lunixi.PaymentMethodPromissoryNote, providerCode, debtorFields).
    WithPromissoryNoteSchedule([]lunixi.PromissoryNoteScheduleRow{
        {Amount: "60000", DueDate: "2027-01-15"},
        {Amount: "60000", DueDate: "2027-02-15"},
    })
// or .WithPromissoryNoteCount(3)
```

Vade tablosu, toplamı ödeme tutarına eşit pozitif küçük birim tutarlı ve artan sıralı `YYYY-MM-DD` takvim vadeli 1–24 satırdan oluşur; tarihin gelecekte olduğunu API denetler. `GetPromissoryNotes` senetleri ve durumlarını döndürür, `CancelPromissoryNotes` listelenen seri numaralarını iptal eder (nil açık senetlerin tümünü iptal eder) ve `DownloadPromissoryNoteDocument` yazdırılabilir PDF'i `Response.Bytes` içinde döndürür (dil `PromissoryNoteDocumentLocaleTR` veya `PromissoryNoteDocumentLocaleEN`). Belge saklanmaz; gerektiğinde indirin. Senet değişiklikleri `PromissoryNoteEvent*` webhook'larıyla gelir. `ADDRESS` alanları kodlarını `client.Reference.ListProvinces`, `ListDistricts(provinceCode)` ve `ListNeighborhoods(districtCode)` çağrılarından alır.

### Para Transferi

```go
request := lunixi.NewDirectMethodPaymentRequest("125000", "TRY", "ORD-1003").
    WithMethodFields(lunixi.PaymentMethodBankTransfer, "", map[string]any{"bankCode": buyerBankCode})
response, err := client.Payments.DirectMethodPayment(ctx, request, persistedAttemptKey)
if err != nil { return err }
if instruction := response.BankTransferInstruction(); instruction != nil {
    // Show IBAN, HolderName, AmountMinor, PaymentCode and ExpiresAt to the buyer.
}
```

Alıcı parayı kendi bankasından sizin banka hesabınıza gönderir; para Lunixi'den geçmez. `bankCode`, `ListPaymentMethods` yanıtındaki yöntem seçeneklerinden gelir; `BankTransferOtherBanksCode` (`"9999"`) listede olmayan bir banka anlamına gelir. Blok sağlayıcı kodu almaz. Talimat gösterim verisidir; ödeme yalnız gateway tahsil edildiğini bildirdiğinde ödenmiş sayılır. Geç, eksik ve fazla gelen transferleri `ListBankTransferExceptions` listeler (`BankTransferExceptionFilters`: `TEST` / `LIVE`, `BankTransferExceptionStatus*`, 1–200 arası limit); `AcceptBankTransferException` ile ya da gönderene iadeyi kendiniz yaptıktan sonra `MarkBankTransferExceptionRefunded` ile karar verin. İlgili webhook'lar `PaymentBankTransferEvent*` olaylarıdır.

## Fiziki POS

`client.PhysicalPos` kendi cihazlarınızda kart ödemelerini başlatır, ters kayıt veya iptal yapar, gün sonunu başlatır; ödemeleri, işlem ağaçlarını, satış ve vergi özetlerini ve gün sonu sonuçlarını okur. `client.PhysicalPosManagement` mağazaları, kasaları, mantıksal terminalleri (banka satırları, ödeme sağlayıcısı satırları, cihaz bağlantıları), cihazları ve kimlik anahtarlarını, hesap, mağaza ve cihaz düzeyindeki gün sonu politikasını yönetir.

```go
key := lunixi.NewPhysicalPosIdempotencyKey()
// Store key with your order BEFORE the call.
result, err := client.PhysicalPos.CreatePayment(ctx, map[string]any{
    "operation":         lunixi.PhysicalPosOperationSale,
    "target":            map[string]any{"deviceId": deviceID},
    "amountMinor":       "4500",
    "currency":          "TRY",
    "merchantReference": orderID,
}, key)
var unknown *lunixi.PhysicalPosOutcomeUnknownError
if errors.As(err, &unknown) {
    // The payment may exist: call again later with the SAME key.
}
// result.StatusCode is 202 for a new payment, 200 with result.Replayed for a repeat.
```

İstekler ve filtreler gateway alan adlarını kullanan map'lerdir (`PhysicalPosRequestFields`, `PhysicalPosFilters`); bilinmeyen alan, eksik zorunlu alan, yanlış tip, sayısal tutar veya 1–100 dışında sayfa `limit` değeri göndermeden önce `ErrConfiguration` döndürür. Tutarlar kanonik küçük birim metindir; adetler, satır numaraları, miktarlar ve `rowVersion` tamsayıdır. `nil` boş bırakılabilir bir alanı temizler (ör. `UpdateStore` içindeki `city`); `""` isteğe bağlı alanı göndermez. `time.Time` filtresi UTC ISO-8601 olarak, `dateFrom` / `dateTo` ise kendi takvim günü olarak gönderilir.

Kart verisi bu API'ye hiçbir zaman gitmez: kart cihazda okunur. İsteğin herhangi bir yerinde kart verisi alan adı (`pan`, `cardNumber`, `cvv`, `expiry`, `track2` …) ya da `merchantReference`, ürün adı, vergi grubu kodu veya `merchantReference` / `q` filtrelerinde kart numarası varsa, değeri değil alan yolunu taşıyan `*PhysicalPosCardDataRejectedError` döner ve hiçbir şey gönderilmez.

Ödeme yazımı ödendi değil kabul edildi demektir: sonucu `PhysicalPosEvent*` webhook'larıyla veya `GetPayment` ile izleyin. `CreatePayment` ve `CreateReversal` anahtar ister ve onu verildiği gibi gönderir (8–200 yazdırılabilir ASCII karakter); `CancelPayment`, `StartSettlement` ve yönetim oluşturma çağrıları `""` kabul eder ve bu durumda yeni bir UUID gönderip `IdempotencyKey` olarak döndürür. Ödeme yazımları kendilerini aynı anahtar ve gövdeyle yeniden dener (`MaxRetries`). Herhangi bir deneme yanıt almadığında, ödemeyi içermeyen bir 2xx ya da hiçbir şeyin başlamadığını söylemeyen bir 5xx geldiğinde çağrı `*PhysicalPosOutcomeUnknownError` döndürür; o ödeme için asla yeni anahtar kullanmayın. Cihazın gün sonundan sonra yapılan iptal `*PhysicalPosVoidAfterSettlementError` olur (bunun yerine iade yapın), 429 `RetryAfter` ile `*PhysicalPosRateLimitError` olur ve anahtarlı bir yazımın diğer hataları anahtarı taşıyan `*PhysicalPosWriteError` olur. Listeler `PhysicalPosPage` döndürür (`Items`, `HasMore`, `NextCursor`, `includeTotal` ile `TotalCount`); her listenin imleci izleyen bir `Iterate…` biçimi de vardır.

`verifier.VerifyPhysicalPos(rawBody, headers)` webhook'u doğrular ve `PhysicalPosWebhookEvent` olarak okur: `Category` (`payment`, `fiscal_document`, `settlement`, `other`), `HistorySeq` (gün sonu olaylarında nil; bunları `ID` ile tekilleştirin), ters kayıtlar için `Original`, cihaz kasiyer oturumunda başlatılan ödemeler için `Cashier` ve doğrudan cihazda yapılan satış için `IsDeviceSale`. Yükteki tutarlar imzadan sonra denetlenir.

## Cüzdan

`client.Wallet`, kapalı devre cüzdanın merchant yüzeyidir ve alanlara göre gruplanır: `Programs`, `EndUsers`, `Wallets`, `Deposits`, `Corporate`, `Fees`, `Transfers` (cüzdandan cüzdana ve IBAN'a), `Withdrawals`, `Operations`, `Topups`, `Payments`, `QR` ve `BulkPayouts`. Her çağrı imzalıdır; anahtarın `WALLET_SERVICE` ürününe ve rotanın gerektirdiği cüzdan yetkisine sahip olması gerekir.

```go
quote, err := client.Wallet.Transfers.QuoteW2W(ctx, map[string]any{
    "endUserId": endUserID, "sourceWalletAccountId": accountID,
    "destinationWalletNo": walletNo, "amount": "12550", "currency": "TRY",
})
started, err := client.Wallet.Transfers.InitiateW2W(ctx, request, key)
// Commit with the integrityHash and nonce of the answer (and otpCode when otpRequired):
done, err := client.Wallet.Transfers.CompleteW2W(ctx, operationID, map[string]any{
    "endUserId": endUserID, "integrityHash": hash, "nonce": nonce, "otpCode": otp,
}, completeKey)
```

İstekler ve filtreler, göndermeden önce `WalletRoutes()` ile (diğer Lunixi SDK'larıyla aynı rota tablosu) denetlenen map'lerdir: bilinmeyen alan, eksik zorunlu alan, küçük birim rakam metni (`"12550"`) olmayan tutar, `ImportEmployees` / `OffboardEmployees` / toplu ödeme `items` içinde hatalı satır ya da anahtar isteyen bir rotada eksik `Idempotency-Key` `ErrConfiguration` döndürür. Cüzdan anahtarları 1–128 yazdırılabilir ASCII karakterdir. `Topups.StartCard` sipariş bilgisi alanlarıyla `buyer` ve `billingAddress` ister (`shippingAddress` isteğe bağlı). `Complete…` çağrıları `operationId` değerini gövdeye kendileri ekler. `QR.Image`, SVG veya PNG'yi `Response.Bytes` içinde döndürür. Listeler `CursorPage` döndürür; `Iterate` imleci izler.

## Banka hesapları, transferler ve döviz

`client.BankConnect` bağlı banka hesaplarınızı okur: `ListBanks`, `ListConnections` / `GetConnection`, `ListAccounts` / `GetAccount`, `ListBalances`, `ListTransactions` / `GetTransaction` (her liste `Iterate…` biçimiyle) ve `RequestSync`. Banka bağlama yalnız Lunixi panelinden yapılır. Filtreler göndermeden önce `BankConnectFilters()` ile denetlenir: yalnız bilinen filtreler, kümelerinden değerler, 1–100 arası `limit`, en fazla 100 hesap kimliği, `minAmountMinor` ≤ `maxAmountMinor` koşuluyla küçük birim metin tutarlar, yalnız `bookedFrom` ile birlikte `bookedTo` ve ikisi arasında en fazla 92 gün.

`client.BankConnect.Transfers` (`ListPurposes`, `Create`, `List` / `Iterate`, `Get`) ve `client.BankConnect.FX` (`ListRates`, `CreateQuote`, `CreateOrder`, `ListOrders` / `IterateOrders`, `GetOrder`) bu özelliği sunan bağlantılarda para hareket ettirir; `bank-connect:transfer` kapsamını ve IP izin listesi olan bir anahtarı ister. Gövdeler `BankTransferRoutes()` ile denetlenir.

```go
result, err := client.BankConnect.Transfers.Create(ctx, map[string]any{
    "accountId": accountID, "receiverIban": iban, "receiverName": "Ada Lovelace",
    "amountMinor": "150075", "paymentPurpose": purposeCode,
}, payoutID) // "" generates a UUID, returned as result.IdempotencyKey
```

`Create` ve `CreateOrder` anahtarınızı denetler (8–200 yazdırılabilir ASCII karakter) ya da bir anahtar üretir; ağ hatasını veya 5xx'i (`MaxRetries`) aynı anahtar ve gövdeyle yeniden dener; aynı gövdeyle aynı anahtar `Replayed` yanıtı verir. Bir döviz teklifi `sourceAmountMinor` / `targetAmountMinor` alanlarından tam olarak birini ve iki farklı hesap alır. Webhook'lar: `ParseBankAccountTransactionReceived`, `BankConnectEventTransactionReceived` olayını; `ParseBankTransferWebhook`, `BankTransferEvent*` olaylarını okur (`bank_transfer.*`; Para Transferi ödemesinin `payment.bank_transfer.*` olayları değildir).

## Yaptırım taraması

`client.Sanction` isimleri, cüzdanları, hesapları, işlemleri, kamu ihalesi yasaklarını ve olumsuz medyayı tarar, davranışsal uyarıları değerlendirir, sürekli izleme kaydeder; uyarıları, eşleşmeleri, izlenen varlıkları, veri seti güncelliğini, varlık istihbaratını ve engelli listenizi okur. Gövdeler ve filtreler göndermeden önce `SanctionRoutes()`, `SanctionNestedFields()` ve `SanctionValueSets()` ile denetlenir.

```go
result, err := client.Sanction.ScreenName(ctx, map[string]any{
    "name": "Ada Lovelace", "subjectType": lunixi.SanctionSubjectTypePerson,
    "riskDomains": []string{lunixi.SanctionRiskDomainSanctions, lunixi.SanctionRiskDomainPep},
})
if err == nil && lunixi.IsClearScreening(result) {
    // nothing found in a COMPLETE screening
}
```

Tarama karar değil eşleşme döndürür. `IsClearScreening` yalnız hiçbir eşleşme, isabet veya engelleyici işaret yoksa ve kapsam bildirildiği yerde `coverageStatus` değeri `COMPLETE` ise true olur; `PARTIAL`, `DEGRADED` veya eksik kapsam hiçbir zaman temiz sayılmaz. Çalıştırılamayan tarama (HTTP 503 ya da durumu `success` olmayan 2xx) boş sonuç değil, `*APIError` olur.

## Sigorta

`client.Insurance` bir sepet için sigorta ürünlerini listeler ve arar, fiyatlar (`CreateQuote`), teklifi siparişe çevirir (`CreateOrder`), prim ödemesini bağlar veya onaylar, poliçe düzenler ve iptal eder, primi iade eder, yalnız talep üzerine satılan türler için potansiyel müşteri gönderir, poliçe belgelerini, makbuzları ve sertifikaları indirir (`Response.Bytes`) ve mutabakatı okur (`ExportReconciliation`, CSV'yi `Truncated`, `NextCursor` ve `Aborted` ile döndürür).

```go
quote, err := client.Insurance.CreateQuote(ctx, map[string]any{
    "insuranceType": "TICKET_CANCELLATION", "currency": "TRY", "paymentIntentId": intentID,
}, "")
consent, err := client.Insurance.ConsentFromQuote(option, acceptedAt)
order, err := client.Insurance.CreateOrder(ctx, map[string]any{"quoteId": quoteID, "consent": consent}, orderKey)
item, err := client.Insurance.PremiumBasketItem(order, "Ticket insurance", "Insurance", "")
```

Alanlar ve filtreler göndermeden önce `InsuranceQuoteFields()`, `InsuranceLeadFields()`, `InsuranceFilters()` ve diğer alan listeleriyle denetlenir; bir sigorta türünün kendi nesnesindeki kurallar gateway'de kalır (bütün alan hatalarıyla tek seferde 422). Teklif sepetini `paymentIntentId` veya `context.items` alanından alır, ikisinden birden değil. `CreateOrder`, `ConfirmPayment`, `IssuePolicy`, `CancelOrder`, `RefundPremium` ve `CreateLead` bir `Idempotency-Key` ister (8–200 yazdırılabilir ASCII karakter); `CreateQuote` ve `BindPayment` `""` kabul eder. Webhook'lar: `ParseInsuranceWebhook` sipariş olaylarını, `ParseInsuranceLeadWebhook` ise `InsuranceEventLeadSubmitted` olayını okur.

## Abonelik

`client.Subscriptions` abonelikleri oluşturur ve yönetir (`Create`, `Get`, `List` / `Iterate`, `Cancel`, `Pause`, `Resume`, `ChangePlan`, `AuditLogs`); ilgili kaynaklar `Plans`, `Customers`, `Cards`, `Invoices`, `Portal`, `Products`, `Features`, `Addons` ve `Usage` altındadır.

```go
customer, err := client.Subscriptions.Customers.Upsert(ctx, map[string]any{
    "externalId": userID, "name": "Ada", "surname": "Yılmaz", "email": email,
    "gsmNumber": "+905000000000", "identityNumber": "11111111111",
    "billingAddress": "Örnek Sok. No: 1", "billingCity": "İstanbul",
    "billingCountry": "Türkiye", "billingZipCode": "34662",
}, "") // yenilemeler alıcıyı bu kayıttan alır
plan, err := client.Subscriptions.Plans.Create(ctx, map[string]any{
    "productId": productID, "code": "pro", "name": "Pro", "price": "9900", "currency": "TRY",
    "billing": map[string]any{"frequency": lunixi.SubscriptionBillingFrequencyMonthly},
}, "")
sub, err := client.Subscriptions.Create(ctx, map[string]any{
    "customerId": customerID, "planId": planID, "paymentCardId": cardID,
}, subscriptionKey) // required; sent in the header and the body
```

İstekler ve filtreler, göndermeden önce `SubscriptionRoutes()` ile (diğer Lunixi SDK'larıyla aynı rota tablosu) denetlenen map'lerdir: bilinmeyen alan veya filtre, eksik zorunlu alan, kümesi dışında bir değer (tahsilat yöntemi, faturalama sıklığı, kart doğrulama modu, plan değişikliği davranışı, portal SDK modu) ya da eksik anahtar, değeri değil alanı adlandıran bir `ErrConfiguration` döndürür. Para alanları gövdelerde, filtrelerde ve yanıtlarda kanonik küçük birim string'lerdir (`"0"` ücretsiz plandır). `Create` bir `Idempotency-Key` ister (1–200 yazdırılabilir ASCII karakter); plan, müşteri, kart, plan değişikliği ve tahsilat yeniden denemesi yazımları isteğe bağlı anahtar alır. `Customers.Upsert`; `externalId`, `name`, `surname`, `email`, `gsmNumber`, `identityNumber`, `billingAddress`, `billingCity`, `billingCountry` ve `billingZipCode` ister (`phone`, `locale` ve `metadata` kabul edilmez); gateway eksik ya da geçersiz müşteri alanına `SubscriptionCustomerErrorCodeFieldRequired` / `SubscriptionCustomerErrorCodeFieldInvalid` döner. `Cards.Add`, checkout SDK'sının `paymentMethodToken` değerini alır; kart numarası reddedilir. Abonelik, plan, müşteri, kart, fatura ve denetim kaydı listeleri, `nextPageToken` değeri `NextCursor` içinde (`Iterate` tarafından `pageToken` olarak geri gönderilir) ve `PageSize` ile `CursorPage` döndürür. `Portal.CreateSdkSession` tarayıcı portal SDK'sı için oturum kanıtını, `Portal.CreatePortalLink` barındırılan bir linki (`SubscriptionPortalURL`) döndürür. `IsSubscriptionActive`, `IsSubscriptionTerminal` ve `SubscriptionEvent*` webhook adları da bu yüzeyle gelir.

## KYC ve kimlik sinyalleri

`client.Kyc` KYC, KYB ve kanıt oturumları oluşturur (`CreateSession`, `EvidenceRequest`), bunları okur ve listeler (`GetSession`, `ListSessions` / `IterateSessions`, `Timeline`), yetki alanını ve birincil belgeyi belirler, sunucunuzdan belge yükler, gönderir, manuel incelemeye yollar; KYC politikanızı, webhook teslimlerinizi ve silme taleplerinizi okur veya günceller. `person` veya `company`, `customerType` ile birlikte `customer` olarak, `metadata` ise `evidenceContextJson` olarak gönderilir; `ttlMinutes` 1–1440 arasıdır. Sonucun belirleyicisi `kyc.*` webhook'udur (`KycEvent*`); `IsKycSessionTerminal` ve `KycAssuranceMeets` kararı okur.

`client.Identity` tarayıcı parmak izi SDK'sının cihaz ve hesap ilişkilendirme sinyallerini okur: `AnalyzeSync` / `Analyze`, `AccountLinks`, `GetSession`, `RelatedSessions`, `Timeline`, `TopSources` ve `ListSessions`. `IsIdentityDeviceShared(links, 2)` iki veya daha fazla hesabın paylaştığı cihazı bildirir; `IdentityLinkReasons` nedenlerini listeler. Bunlar izin / engelleme kararı değil, sinyaldir.

## Pazaryeri alt bayileri

`client.Marketplace` tahsilatları alt bayiler arasında böler; alt bayi yaşam döngüsünü, komisyonları, hakediş ödemelerini ve denetim kaydını yürütür.

```go
legs := []lunixi.MarketplaceSeller{{SubDealerID: "d_alice", AmountMinor: "6050"}, {SubDealerID: "d_bob", AmountMinor: "4000"}}
intent, err := client.Marketplace.CreateCheckoutIntent(ctx, intentRequest, legs, lunixi.MarketplaceMultiSellerPolicyAllOrNothing, attemptKey) // intentRequest sipariş bilgileriyle
dealer, err := client.Marketplace.CreateDealer(ctx, map[string]any{"legalName": "Acme Ltd", "dealerType": lunixi.MarketplaceDealerTypeLegalEntity})
link, err := client.Marketplace.OnboardingToken(ctx, dealerID)
onboardingURL := lunixi.MarketplaceOnboardingURL(link, "")
```

Bölünmüş tahsilatlar (`CreateCheckoutIntent`, `ChargeMarketplaceCard2d`, `ChargeMarketplaceCard3d`, `ChargeMarketplaceCard`) imzalıdır ve isteğe bağlı bir ödeme anahtarı alır (16–128 yazdırılabilir ASCII karakter). Satıcı parçalarının pozitif küçük birim paylarla toplamı `amount` değerine eşit olmalıdır; kart tahsilatı ayrıca `currency`, `orderId`, `card`, `buyer` (`ip` dahil), `billingAddress` ve en az bir sepet kalemi (`PHYSICAL` kalemde `shippingAddress`) ister, her biri sipariş bilgisi alanlarıyla; `price` / `paidPrice` alanlarını ve boolean olmayan `preauth` değerini reddeder. Kart verisi verildiği gibi gönderilir ve hiçbir hata mesajında görünmez; çoğu pazaryeri barındırılan `CreateCheckoutIntent` akışını kullanır; yanıt token taşımazsa bu çağrı `*APIError` döndürür. Bayi, komisyon, hakediş ödemesi ve denetim rotaları yalnız erişim token'ıyla çalışır ve `MarketplaceRoutes()` ile denetlenir; para alanları gövdelerde, filtrelerde ve yanıtlarda kanonik küçük birim string'lerdir.

## Güvenli tekrar ve webhook

Finansal mutasyon yardımcıları 16–128 karakterlik (ödeme linkleri için 8–200), boşluksuz yazdırılabilir ASCII bir idempotency anahtarı ister. Anahtar ödeme denemesi başlamadan kaydedilir; zaman aşımında aynı gövde ve anahtar kullanılır. Varsayılan otomatik tekrar sıfırdır. `MaxRetries` yalnız GET veya anahtarlı isteklerde geçerlidir; anahtarsız mutasyon tekrarlanmaz. Bilinmeyen sonucu yeni ödeme açmadan önce uzlaştırın.

```go
verifier, err := lunixi.NewWebhookVerifier(lunixi.WebhookConfig{
    Secrets: []string{os.Getenv("LUNIXI_WEBHOOK_SECRET")},
})
if err != nil { return err }
event, err := verifier.VerifyRequest(ctx, request)
if err != nil { return err }
inbox, err := lunixi.NewSQLInbox(db,"lunixi_webhook_inbox",merchantScope)
if err != nil { return err }
// EnsureSchema başlangıçta veya migration sırasında bir kez çalıştırılır.
applied, err := inbox.Process(ctx,event,func(tx *sql.Tx)error {
    return dogrulanmisSiparisiIsle(ctx,tx,event)
})
```

`dogrulanmisSiparisiIsle` uygulamanızın sipariş eşleştirmesi ve durum geçişidir. İşlem kaydı ve sipariş güncellemesi **aynı transaction** içinde olmalıdır. Callback başarısızsa event kaydı da geri alınır; yeniden teslim güvenle işlenebilir. Tekrar gelen olay callback'i çalıştırmaz. Aynı event ID farklı gövdeyle gelirse reddedilir. Harici yan etkiler için transaction içi outbox kullanın. Tekrar kayıtlarını sağlayıcının teslim/replay politikasına uygun süre saklayın.

İmza tam ham gövde + event ID + zaman damgasına bağlı HMAC-SHA256'tır. Varsayılan zaman toleransı 5 dakikadır; en çok 15 dakika ayarlanabilir, kapatılamaz. En fazla üç secret ile rotasyon desteklenir. Ham gövdeyi JSON middleware'den önce doğrulayın. İmzalı gövdede `type` gereklidir; imzasız başlık tek başına olay türünü belirlemez. Token, özel anahtar, müşteri bilgileri, ham istek ve webhook gövdelerini loglamayın. `APIError` yalnız durum, güvenli kod ve request ID taşır.

[SQLite webhook örneği](../examples/webhook) gerçek dosya üzerinde atomik geri alma, yeniden başlatma, eşzamanlı tekrar ve HTTP doğrulama testleri içerir. Örnek yalnız doğrulanmış bildirim kaydeder, siparişi tamamlamaz. Sipariş adaptörünüz finansal alanları denetlemelidir. HTTP dinleyicisini güvenilir HTTPS proxy arkasında kullanın.

## Kontroller

```sh
go test -race ./...
go vet ./...
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
cd examples/webhook
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

SQLite örneği CGO ve C derleyicisi ister; ana SDK istemez. Testler merchant anahtarı kullanmaz; TLS sunucuları ve sentetik çapraz dil imza vektörleriyle çalışır. Güvenlik bildirimleri için [SECURITY.md](../SECURITY.md), katkı kuralları için [CONTRIBUTING.md](../CONTRIBUTING.md) dosyasına bakın.

## Mağaza içi entegrasyon örneği

Kaynak örnekteki `RenderCheckoutHTML(token, nonce)` ve `HostedCheckoutURL(paymentPageUrl)` yardımcıları [presentation.go](https://github.com/Lunixi-Payment/lunixi-go/blob/main/examples/checkout/presentation.go) dosyasındadır. Kendi yetkili ve CSRF korumalı mağaza controller’ınıza uyarlayın. CLI varsayılanı `LUNIXI_CHECKOUT_MODE=embedded`; `hosted` backend adresini doğrular. HTML, token veya URL konsola basılmaz. Bunlar yeni SDK istemci yöntemleri değil, kaynak örnek yardımcılarıdır.
