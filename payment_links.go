package lunixi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	paymentLinkImagesPath = paymentLinksPath + "/images"
	paymentLinkQRAccept   = "image/svg+xml, image/png, application/json"
	storageErrorBodyLimit = 64 << 10
)

var storageErrorCodePattern = regexp.MustCompile(`<Code>([A-Za-z0-9.]{1,64})</Code>`)

// PaymentLinkClient manages payment links (/api/v1/payments/links). Every
// gateway call is signed with the client key, reads included; the merchant and
// the environment come from that key, so a TEST key sees and changes only TEST
// links. The key needs the matching scope (payment-link:create, :view,
// :manage, :send); a missing scope is 403 API_INSUFFICIENT_SCOPE.
//
// The only request that is not sent through the signed transport is the
// image PUT of UploadImage: it goes to the storage host behind a presigned URL
// and carries only the upload headers, never the access token or signature.
type PaymentLinkClient struct{ client *Client }

// PaymentLinkPage holds the paging filters of the list calls. Limit 0 and an
// empty Cursor are left out; pass the previous page's pageInfo.nextCursor as
// Cursor. IncludeTotal asks for pageInfo.totalCount.
type PaymentLinkPage struct {
	Limit        int
	Cursor       string
	IncludeTotal bool
}

// PaymentLinkListFilters filters PaymentLinks.List. State, Usage and
// AmountMode accept several values (sent as a comma list). CreatedFrom and
// CreatedTo (RFC 3339) bound a half-open window [from, to). Query matches a
// short code exactly, or a title or reference prefix.
type PaymentLinkListFilters struct {
	PaymentLinkPage
	State       []string
	Usage       []string
	AmountMode  []string
	Environment string
	Tag         string
	BranchCode  string
	CreatedByID string
	Query       string
	CreatedFrom string
	CreatedTo   string
}

// PaymentLinkAttemptFilters filters PaymentLinks.ListAttempts; State takes
// PaymentLinkAttemptState* values.
type PaymentLinkAttemptFilters struct {
	PaymentLinkPage
	State []string
}

// PaymentLinkImageUpload is the upload target of CreateImageUpload: PUT the
// file's bytes to UploadURL with exactly UploadHeaders before ExpiresAt, then
// call ConfirmImageUpload(AssetID). UploadURL is the storage host, not the
// gateway: send it no access token and no signature.
type PaymentLinkImageUpload struct {
	AssetID       string            `json:"assetId"`
	UploadURL     string            `json:"uploadUrl"`
	UploadMethod  string            `json:"uploadMethod"`
	UploadHeaders map[string]string `json:"uploadHeaders"`
	ExpiresAt     string            `json:"expiresAt"`
}

// PaymentLinkImage is a published payment link image. Use AssetID as the
// ImageAssetID of a link or an item. URL is empty when the public image host
// is not configured.
type PaymentLinkImage struct {
	AssetID string `json:"assetId"`
	URL     string `json:"url"`
	State   string `json:"state"`
}

func (p PaymentLinkPage) query(values url.Values) error {
	if p.Limit < 0 {
		return invalid("limit must be a positive integer")
	}
	if p.Limit > 0 {
		values.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		values.Set("cursor", p.Cursor)
	}
	if p.IncludeTotal {
		values.Set("includeTotal", "true")
	}
	return nil
}

func listQuery(values url.Values, name string, list, allowed []string) error {
	for _, value := range list {
		if e := oneOf(value, allowed, name); e != nil {
			return e
		}
	}
	if len(list) > 0 {
		values.Set(name, strings.Join(list, ","))
	}
	return nil
}

func (f PaymentLinkListFilters) query() (url.Values, error) {
	values := url.Values{}
	if e := f.PaymentLinkPage.query(values); e != nil {
		return nil, e
	}
	for _, list := range []struct {
		name           string
		given, allowed []string
	}{{"state", f.State, paymentLinkStates}, {"usage", f.Usage, paymentLinkUsages}, {"amountMode", f.AmountMode, paymentLinkAmountModes}} {
		if e := listQuery(values, list.name, list.given, list.allowed); e != nil {
			return nil, e
		}
	}
	if f.Environment != "" {
		if e := oneOf(f.Environment, paymentLinkEnvironments, "environment"); e != nil {
			return nil, e
		}
	}
	for name, value := range map[string]string{"environment": f.Environment, "tag": f.Tag, "branchCode": f.BranchCode, "createdById": f.CreatedByID, "query": f.Query, "createdFrom": f.CreatedFrom, "createdTo": f.CreatedTo} {
		if value != "" {
			values.Set(name, value)
		}
	}
	return values, nil
}

func paymentLinkPath(linkID string) (string, error) {
	id, err := idPath(linkID)
	if err != nil {
		return "", invalid("linkId")
	}
	return paymentLinksPath + "/" + id, nil
}

func requirePaymentLinkKey(key string) error {
	if !validPaymentLinkIdempotency(key) {
		return invalid("stable IdempotencyKey required (8–200 printable ASCII characters without spaces)")
	}
	return nil
}

func optionalPaymentLinkKey(key string) error {
	if key == "" {
		return nil
	}
	return requirePaymentLinkKey(key)
}

// Create creates a payment link. key is required: repeating the same key with
// the same body returns the stored link with replayed true (HTTP 200) instead
// of a second link; the same key with another body is 409
// payment_link.idempotency.conflict.
func (l *PaymentLinkClient) Create(ctx context.Context, request PaymentLinkRequest, key string) (*Response, error) {
	body, err := request.body()
	if err != nil {
		return nil, err
	}
	if err = requirePaymentLinkKey(key); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "POST", Path: paymentLinksPath, Body: body, IdempotencyKey: key})
}

// List returns one page of links, newest first; Data.items holds the rows and
// PageInfo the paging.
func (l *PaymentLinkClient) List(ctx context.Context, filters PaymentLinkListFilters) (*Response, error) {
	query, err := filters.query()
	if err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "GET", Path: paymentLinksPath, Query: query})
}

// Get reads one link.
func (l *PaymentLinkClient) Get(ctx context.Context, linkID string) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "GET", Path: path})
}

// Update changes a link and creates a new version of it (PATCH). Only the
// keys in changes are updated (PaymentLinkSpecFields names); a nil value
// clears a field and a nested object is replaced as a whole. Usage, amountMode
// and currency cannot change. expectedRowVersion is the link's current
// rowVersion; a concurrent change answers 409
// payment_link.link.version_conflict. Amount fields are locked (409
// payment_link.link.amount_locked) while a payment is in progress or once the
// link has been paid. key is optional ("" sends none).
func (l *PaymentLinkClient) Update(ctx context.Context, linkID string, expectedRowVersion int64, changes map[string]any, key string) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	body, err := paymentLinkUpdateBody(expectedRowVersion, changes)
	if err != nil {
		return nil, err
	}
	if err = optionalPaymentLinkKey(key); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "PATCH", Path: path, Body: body, IdempotencyKey: key})
}

// Pause stops accepting payments until Resume.
func (l *PaymentLinkClient) Pause(ctx context.Context, linkID string, options PaymentLinkActionOptions, key string) (*Response, error) {
	return l.Action(ctx, linkID, "pause", options, key)
}

// Resume accepts payments again after Pause.
func (l *PaymentLinkClient) Resume(ctx context.Context, linkID string, options PaymentLinkActionOptions, key string) (*Response, error) {
	return l.Action(ctx, linkID, "resume", options, key)
}

// Deactivate closes the link permanently; it cannot be reopened.
func (l *PaymentLinkClient) Deactivate(ctx context.Context, linkID string, options PaymentLinkActionOptions, key string) (*Response, error) {
	return l.Action(ctx, linkID, "deactivate", options, key)
}

// Action runs "pause", "resume" or "deactivate" on a link. key is optional.
func (l *PaymentLinkClient) Action(ctx context.Context, linkID, action string, options PaymentLinkActionOptions, key string) (*Response, error) {
	if !slices.Contains(paymentLinkActions, action) {
		return nil, invalid("payment link action must be one of " + strings.Join(paymentLinkActions, ", "))
	}
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	if v := options.ExpectedRowVersion; v != nil && (*v < 0 || *v > MaxSafeInteger) {
		return nil, invalid("expectedRowVersion must be the link's current rowVersion (a non-negative integer)")
	}
	if err = optionalPaymentLinkKey(key); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "POST", Path: path + "/" + action, Body: options, IdempotencyKey: key})
}

// Send delivers the link by e-mail or SMS. key is required so a retried call
// cannot send twice. An accepted delivery can still fail: check
// data.delivery.state and data.delivery.failureReason (with a TEST key only
// your organisation's own verified addresses receive messages).
func (l *PaymentLinkClient) Send(ctx context.Context, linkID string, request PaymentLinkSendRequest, key string) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	if err = request.validate(); err != nil {
		return nil, err
	}
	if err = requirePaymentLinkKey(key); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "POST", Path: path + "/send", Body: request, IdempotencyKey: key})
}

// QR returns the link's QR code image in Response.Bytes (Response.ContentType
// image/svg+xml or image/png). format is PaymentLinkQRFormatSVG (also for "")
// or PaymentLinkQRFormatPNG; size 0 uses the default (512), otherwise
// 128–2048 pixels.
func (l *PaymentLinkClient) QR(ctx context.Context, linkID, format string, size int) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = PaymentLinkQRFormatSVG
	}
	if err = oneOf(format, paymentLinkQRFormats, "QR format"); err != nil {
		return nil, err
	}
	query := url.Values{"format": {format}}
	if size != 0 {
		if size < PaymentLinkQRMinSize || size > PaymentLinkQRMaxSize {
			return nil, invalid("QR size must be 128–2048 pixels")
		}
		query.Set("size", strconv.Itoa(size))
	}
	return l.client.do(ctx, Request{Method: "GET", Path: path + "/qr", Query: query}, paymentLinkQRAccept)
}

// ListAttempts lists the payment attempts on a link, newest first.
func (l *PaymentLinkClient) ListAttempts(ctx context.Context, linkID string, filters PaymentLinkAttemptFilters) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if err = filters.PaymentLinkPage.query(query); err != nil {
		return nil, err
	}
	if err = listQuery(query, "state", filters.State, paymentLinkAttemptStates); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "GET", Path: path + "/attempts", Query: query})
}

// ListPayments lists the successful payments on a link, with refund totals.
func (l *PaymentLinkClient) ListPayments(ctx context.Context, linkID string, page PaymentLinkPage) (*Response, error) {
	path, err := paymentLinkPath(linkID)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if err = page.query(query); err != nil {
		return nil, err
	}
	return l.client.Do(ctx, Request{Method: "GET", Path: path + "/payments", Query: query})
}

// CreateImageUpload is step 1 of 3 of a link image: an upload target for a
// PNG, JPEG or WebP image (POST /payments/links/images). fileName (1–255
// characters) is recorded only; a declared sizeBytes over the 2 MiB image
// limit is refused by the gateway before anything is uploaded. Step 2 is a PUT
// of the bytes to UploadURL with exactly UploadHeaders; step 3 is
// ConfirmImageUpload. UploadImage runs all three.
func (l *PaymentLinkClient) CreateImageUpload(ctx context.Context, fileName, contentType string, sizeBytes int64) (*PaymentLinkImageUpload, error) {
	if strings.TrimSpace(fileName) == "" || utf8.RuneCountInString(fileName) > 255 {
		return nil, invalid("fileName must be 1–255 characters")
	}
	if err := oneOf(contentType, paymentLinkImageTypes, "image contentType (SVG is not accepted)"); err != nil {
		return nil, err
	}
	if sizeBytes < 1 || sizeBytes > MaxSafeInteger {
		return nil, invalid("image sizeBytes must be the file size in bytes")
	}
	body := orderedObject{{key: "fileName", value: fileName}, {key: "contentType", value: contentType}, {key: "sizeBytes", value: sizeBytes}}
	response, err := l.client.Do(ctx, Request{Method: "POST", Path: paymentLinkImagesPath, Body: body})
	if err != nil {
		return nil, err
	}
	var target PaymentLinkImageUpload
	if json.Unmarshal(response.Data, &target) != nil || !target.usable() {
		return nil, &APIError{StatusCode: response.StatusCode, Code: "INVALID_UPLOAD_TARGET", RequestID: response.RequestID}
	}
	return &target, nil
}

// usable reports whether the SDK can PUT to the target as the contract
// describes; anything else fails before a byte is sent.
func (t PaymentLinkImageUpload) usable() bool {
	if !uuidPattern.MatchString(t.AssetID) || t.UploadMethod != http.MethodPut || t.UploadHeaders == nil {
		return false
	}
	u, err := url.Parse(t.UploadURL)
	return err == nil && slices.Contains(paymentLinkUploadSchemes, u.Scheme) && u.Host != "" && u.User == nil
}

// ConfirmImageUpload is step 3 of 3: it publishes an uploaded image
// (POST /payments/links/images/{assetId}/confirm). The gateway checks the
// stored bytes (declared type, at most 2 MiB and 4096×4096 px); a file that
// fails is deleted and must be uploaded again from CreateImageUpload.
// Confirming before the PUT has landed is 400 media.confirm.object_missing and
// can be retried.
func (l *PaymentLinkClient) ConfirmImageUpload(ctx context.Context, assetID string) (*PaymentLinkImage, error) {
	if !uuidPattern.MatchString(assetID) {
		return nil, invalid("assetId must be the image's asset id (uuid)")
	}
	response, err := l.client.Do(ctx, Request{Method: "POST", Path: paymentLinkImagesPath + "/" + assetID + "/confirm", Body: map[string]any{}})
	if err != nil {
		return nil, err
	}
	var image PaymentLinkImage
	if json.Unmarshal(response.Data, &image) != nil || image.AssetID == "" || image.State != paymentLinkImagePublished {
		return nil, &APIError{StatusCode: response.StatusCode, Code: "IMAGE_NOT_PUBLISHED", RequestID: response.RequestID}
	}
	return &image, nil
}

// UploadImage uploads a link image in one call: CreateImageUpload, the PUT of
// content to the returned UploadURL, then ConfirmImageUpload. The PUT goes
// straight to the storage host with only the upload headers, through the
// configured HTTP client but not the signed transport, so the access token and
// the request signature are never sent there. A failed PUT is not retried: it
// returns an APIError with the storage host's HTTP status and error code (for
// example AccessDenied once the URL has expired); call UploadImage again for a
// new upload URL.
func (l *PaymentLinkClient) UploadImage(ctx context.Context, fileName, contentType string, content []byte) (*PaymentLinkImage, error) {
	if len(content) == 0 {
		return nil, invalid("image content must not be empty")
	}
	target, err := l.CreateImageUpload(ctx, fileName, contentType, int64(len(content)))
	if err != nil {
		return nil, err
	}
	if err = l.client.putToStorage(ctx, target, content); err != nil {
		return nil, err
	}
	return l.ConfirmImageUpload(ctx, target.AssetID)
}

// putToStorage sends the presigned PUT. It deliberately bypasses exchange: no
// Authorization, X-Key-Id, X-Date, X-Nonce, X-Signature or Digest header is
// added, only the headers the URL was signed for.
func (c *Client) putToStorage(ctx context.Context, target *PaymentLinkImageUpload, content []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.UploadURL, bytes.NewReader(content))
	if err != nil {
		return invalid("image upload URL")
	}
	for name, value := range target.UploadHeaders {
		req.Header.Set(name, value)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &APIError{Code: "STORAGE_TRANSPORT_ERROR"}
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, storageErrorBodyLimit))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	code := "STORAGE_UPLOAD_FAILED"
	if match := storageErrorCodePattern.FindSubmatch(raw); match != nil {
		code = string(match[1])
	}
	return &APIError{StatusCode: response.StatusCode, Code: code}
}
