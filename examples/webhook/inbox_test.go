package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	lunixi "github.com/Lunixi-Payment/lunixi-go"
)

const testSecret = "whsec_synthetic_test_secret_123456789"

func signed(t *testing.T, id, raw string) (*lunixi.WebhookVerifier, *lunixi.WebhookEvent, http.Header) {
	t.Helper()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(id + "." + ts + "." + raw))
	h := http.Header{}
	h.Set("X-Lunixi-Event-Id", id)
	h.Set("X-Lunixi-Signature-Timestamp", ts)
	h.Set("X-Lunixi-Signature", "v2="+hex.EncodeToString(mac.Sum(nil)))
	v, err := lunixi.NewWebhookVerifier(lunixi.WebhookConfig{Secrets: []string{testSecret}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := v.Verify([]byte(raw), h)
	if err != nil {
		t.Fatal(err)
	}
	return v, e, h
}
func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, e := sql.Open("sqlite3", "file:"+path+"?_busy_timeout=5000&_journal_mode=WAL")
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
func inboxFor(t *testing.T, db *sql.DB, scope string) *lunixi.SQLInbox {
	t.Helper()
	i, e := lunixi.NewSQLInbox(db, "lunixi_inbox", scope)
	if e != nil {
		t.Fatal(e)
	}
	if e = i.EnsureSchema(context.Background()); e != nil {
		t.Fatal(e)
	}
	return i
}
func TestSQLInboxAtomicRollbackDurabilityAndCollision(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	db := openDB(t, path)
	inbox := inboxFor(t, db, "merchant-a")
	if _, e := db.Exec("CREATE TABLE orders (id TEXT PRIMARY KEY, applications INTEGER NOT NULL); INSERT INTO orders VALUES ('order-1',0)"); e != nil {
		t.Fatal(e)
	}
	_, event, _ := signed(t, "event-1", `{"type":"payment.captured","data":{"orderId":"order-1","capturedAmountMinor":"10050","currency":"TRY"}}`)
	crash := errors.New("synthetic crash")
	applied, e := inbox.Process(ctx, event, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE orders SET applications=applications+1 WHERE id='order-1'"); err != nil {
			return err
		}
		return crash
	})
	if applied || !errors.Is(e, crash) {
		t.Fatalf("failed transaction: %v %v", applied, e)
	}
	apply := func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE orders SET applications=applications+1 WHERE id='order-1'")
		return err
	}
	applied, e = inbox.Process(ctx, event, apply)
	if !applied || e != nil {
		t.Fatalf("retry failed: %v %v", applied, e)
	}
	applied, e = inbox.Process(ctx, event, apply)
	if applied || e != nil {
		t.Fatalf("duplicate: %v %v", applied, e)
	}
	var count int
	if e = db.QueryRow("SELECT applications FROM orders").Scan(&count); e != nil || count != 1 {
		t.Fatalf("atomic update: %d %v", count, e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	db = openDB(t, path)
	inbox = inboxFor(t, db, "merchant-a")
	applied, e = inbox.Process(ctx, event, apply)
	if applied || e != nil {
		t.Fatalf("restart duplicate: %v %v", applied, e)
	}
	_, collision, _ := signed(t, "event-1", `{"type":"payment.failed","data":{}}`)
	if _, e = inbox.Process(ctx, collision, apply); !errors.Is(e, lunixi.ErrWebhook) {
		t.Fatalf("collision: %v", e)
	}
	other := inboxFor(t, db, "merchant-b")
	applied, e = other.Process(ctx, event, func(*sql.Tx) error { return nil })
	if !applied || e != nil {
		t.Fatalf("scope: %v %v", applied, e)
	}
}
func TestSQLInboxConcurrentDuplicateAndHTTP(t *testing.T) {
	ctx := context.Background()
	db := openDB(t, filepath.Join(t.TempDir(), "http.db"))
	inbox := inboxFor(t, db, "merchant-a")
	if _, e := db.Exec("CREATE TABLE payment_notifications (event_id TEXT PRIMARY KEY,event_type TEXT NOT NULL)"); e != nil {
		t.Fatal(e)
	}
	raw := `{"type":"payment.captured","data":{}}`
	v, event, h := signed(t, "event-concurrent", raw)
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, e := inbox.Process(ctx, event, func(*sql.Tx) error { return nil })
			if e != nil {
				t.Error(e)
			}
			if ok {
				mu.Lock()
				applied++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if applied != 1 {
		t.Fatalf("commits=%d", applied)
	}
	_, _, h = signed(t, "event-http", raw)
	handler := receiptHandler(v, inbox)
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/webhooks/lunixi", strings.NewReader(raw))
		r.Header = h.Clone()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
	}
	var count int
	if e := db.QueryRow("SELECT COUNT(*) FROM payment_notifications").Scan(&count); e != nil || count != 1 {
		t.Fatalf("HTTP notification count %d %v", count, e)
	}
	r := httptest.NewRequest("POST", "/webhooks/lunixi", strings.NewReader(raw+" "))
	r.Header = h.Clone()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("tamper HTTP=%d", w.Code)
	}
}
