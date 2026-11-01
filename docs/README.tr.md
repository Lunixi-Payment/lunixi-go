# Lunixi Go SDK

**1.0.0**, kanonik küçük birim string para sözleşmesini kullanır. [Geçiş rehberini](../MIGRATION-1.0.md) uygulayın; herkese açık sürüm etiketi kullanılabilir olduğunda aşağıdaki komutla yükleyin. `go.sum` dosyasını saklayın ve modülü Go proxy/checksum veritabanıyla doğrulayın.

1.0.0 sürümü mağaza sitesi için somut bir sunum örneği ekler. **Varsayılan gömülü ödemedir**: yalnız resmi Lunixi bundle yüklenir ve mevcut form mount edilir; yöntem ve çoklu kart seçenekleri Lunixi panelinde kalır. İsteğe bağlı yönlendirme yalnız backend `paymentPageUrl` adresini, tam HTTPS `pay.lunixi.com/payment/` doğrulamasından sonra kullanır. Adresin token’ı gömülü token’dan farklı olabilir. Eksik adres gömülü ödemeyi engellemez; yönlendirme aynı deneme/anahtarla inceleme veya uzlaştırmada kalır. Tarayıcı token’ından adres üretmeyin. Formdan önce müşteri/sipariş sahipliğini ve yerel tarayıcı CSRF’yi doğrulayın; `Cache-Control: private, no-store` gönderin, mağazanın CSP nonce değerini koruyun; token, HTML ve yönlendirme adreslerini loglamayın. Genel yanıt verisi `paymentPageUrl` alanını korur; yeni tipli ürün eşitliği iddiası yoktur.

[English README](../README.md) · [Go dokümantasyonu](https://docs.lunixi.com/tr/sdks/go)

Go 1.26+ sunucu uygulamaları için Lunixi istemcisidir. Çalışma zamanı bağımlılığı yoktur; `net/http`, Gin, Echo ve Fiber ile kullanılabilir.

```sh
go get github.com/Lunixi-Payment/lunixi-go@v1.0.0
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
    BasketItems: []lunixi.BasketItem{
        {ID:"line-1", Name:"Örnek ürün", ItemType:"PHYSICAL", Price:"10050"},
    },
}, kayitliDenemeAnahtari)
```

`lunixi` import yolu `github.com/Lunixi-Payment/lunixi-go` şeklindedir. İstemciyi her merchant anahtarı için bir kez oluşturup paylaşın. SDK Ed25519 imzasını, token önbelleğini ve eşzamanlı yenilemeyi yönetir. Özel anahtar sunucuda kalır; PKCS8 PEM / base64 DER / base64 Ed25519 seed kabul edilir. TEST/LIVE ortamını merchant anahtarı belirler. BaseURL güvenilir HTTPS kökeni olmalıdır; yönlendirmeler izlenmez.

`Amount` ve sepet `Price` alanları kanonik alt birim string değeridir: `"10050"` = 100,50 TRY. Sepet toplamı `Amount` ile aynı olmalıdır. Kaldırılmış kök alanlar `price`, `paidPrice`, `autoCapture` gönderilmez. Taksit ve vade farkını Lunixi formu yönetir. Token yalnız ilgili siparişin yetkili müşterisine `Cache-Control: no-store` ile verilir; loglanmaz. PAN/CVV bu örneklere girmez.

## Kapsam

- `Payments`: intent, sorgulama, listeleme, yöntemler, capture, refund, void, split ödeme ve split iadesi.
- `Fraud`: karar değerlendirme, olay gönderme ve karar sorguları.
- `Client.Do`: diğer belgelenmiş `/api/v1/` uçlarına aynı imzalı taşıma katmanıyla erişim. Sigorta, cüzdan, banka, pazaryeri vb. için bu sürümde ayrı tipli yardımcılar yoktur; REST şeması izlenir.
- `WebhookVerifier` ve `SQLInbox`: ham gövde v2 imzası ve kalıcı transaction içinde tekrar engelleme.

Yanıtta `Data`, `PageInfo`, asıl zarf ve ikili `Bytes` korunur. `DecodeData` arayüz değerleri için `json.Number` kullanır. Split bekleyen durumlarda HTTP 400 içeren `Response` dönebilir; bu tahsilat değildir. Siparişi yalnız imzalı webhook / yetkili sunucu sorgusunda sipariş, merchant, ödeme kimliği, tutar, para birimi ve nihai tahsilat durumunu eşleştirerek tamamlayın. Tarayıcı dönüşü, HTTP 200 veya `AUTHORIZED` ödeme kanıtı değildir.

## Güvenli tekrar ve webhook

Finansal mutasyon yardımcıları 16–128 karakterlik, boşluksuz yazdırılabilir ASCII bir idempotency anahtarı ister. Anahtar ödeme denemesi başlamadan kaydedilir; zaman aşımında aynı gövde ve anahtar kullanılır. Varsayılan otomatik tekrar sıfırdır. `MaxRetries` yalnız GET veya anahtarlı isteklerde geçerlidir; anahtarsız mutasyon tekrarlanmaz. Bilinmeyen sonucu yeni ödeme açmadan önce uzlaştırın.

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
