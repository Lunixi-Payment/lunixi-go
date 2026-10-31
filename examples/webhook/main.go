// This runnable receipt server verifies webhooks and writes a durable
// notification in the same transaction as the replay marker. It deliberately
// does not mark orders paid: add your validated order adapter in that callback.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	lunixi "github.com/Lunixi-Payment/lunixi-go"
	_ "github.com/mattn/go-sqlite3"
)

func receiptHandler(verifier *lunixi.WebhookVerifier, inbox *lunixi.SQLInbox) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		event, err := verifier.VerifyRequest(ctx, r)
		if err != nil {
			http.Error(w, "invalid webhook", http.StatusBadRequest)
			return
		}
		_, err = inbox.Process(ctx, event, func(tx *sql.Tx) error {
			// Notification and replay marker both commit or both roll back. Use an
			// outbox for network effects; never charge or email inside this transaction.
			_, err := tx.ExecContext(ctx, "INSERT INTO payment_notifications (event_id,event_type) VALUES (?,?)", event.ID(), event.Type())
			return err
		})
		if errors.Is(err, lunixi.ErrWebhook) {
			http.Error(w, "event collision", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func run() error {
	verifier, err := lunixi.NewWebhookVerifier(lunixi.WebhookConfig{Secrets: []string{os.Getenv("LUNIXI_WEBHOOK_SECRET")}})
	if err != nil {
		return err
	}
	// A local durable file, not :memory:. Bind only loopback behind a trusted
	// HTTPS reverse proxy. Never expose this plain HTTP listener directly.
	db, err := sql.Open("sqlite3", "file:lunixi-webhooks.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inbox, err := lunixi.NewSQLInbox(db, "lunixi_webhook_inbox", "merchant-payments")
	if err != nil {
		return err
	}
	if err = inbox.EnsureSchema(ctx); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS payment_notifications (event_id TEXT PRIMARY KEY,event_type TEXT NOT NULL)"); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/webhooks/lunixi", receiptHandler(verifier, inbox))
	server := &http.Server{Addr: "127.0.0.1:8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	return server.ListenAndServe()
}

func main() {
	if err := run(); err != nil {
		log.Fatal("webhook server stopped; verify configuration and database access")
	}
}
