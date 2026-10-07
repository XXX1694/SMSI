// Command telegram manages the Telegram side of the deployment.
//
//	telegram set-webhook [-url URL] [-drop-pending]   register the webhook (TELEGRAM_UPDATES_MODE=webhook)
//	telegram delete-webhook [-drop-pending]           remove it, e.g. before switching to polling
//	telegram webhook-info                             show what Telegram has registered
//
// It reads TELEGRAM_BOT_TOKEN, TELEGRAM_WEBHOOK_SECRET and API_PUBLIC_URL like the
// other binaries. The webhook URL defaults to ${API_PUBLIC_URL}/api/v1/webhooks/telegram
// and must be https; Telegram sends TELEGRAM_WEBHOOK_SECRET back in the
// X-Telegram-Bot-Api-Secret-Token header of every request.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/telegram"
	"github.com/socialos/backend/internal/config"
)

// webhookPath is the API route that receives Telegram updates.
const webhookPath = "/api/v1/webhooks/telegram"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "telegram:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: telegram set-webhook|delete-webhook|webhook-info")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.TelegramToken == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set")
	}
	bot := telegram.New(telegram.Config{BotToken: cfg.TelegramToken})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	url := fs.String("url", "", "webhook URL (default API_PUBLIC_URL"+webhookPath+")")
	drop := fs.Bool("drop-pending", false, "discard updates Telegram has queued")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "set-webhook":
		target := *url
		if target == "" {
			target = strings.TrimRight(cfg.APIPublicURL, "/") + webhookPath
		}
		if cfg.TelegramWebhookSecret == "" {
			return errors.New("TELEGRAM_WEBHOOK_SECRET is not set (openssl rand -hex 32)")
		}
		if err := bot.SetWebhook(ctx, target, cfg.TelegramWebhookSecret, *drop); err != nil {
			return err
		}
		fmt.Println("webhook set:", target)
		if cfg.TelegramUpdatesMode != config.TelegramModeWebhook {
			fmt.Println("note: TELEGRAM_UPDATES_MODE is", cfg.TelegramUpdatesMode, "- set it to webhook so the API accepts the deliveries and the worker stops polling")
		}
	case "delete-webhook":
		if err := bot.DeleteWebhook(ctx, *drop); err != nil {
			return err
		}
		fmt.Println("webhook deleted; polling can be used")
	case "webhook-info":
		info, err := bot.GetWebhookInfo(ctx)
		if err != nil {
			return err
		}
		if info.URL == "" {
			fmt.Println("no webhook is set")
		} else {
			fmt.Println("url:", info.URL)
		}
		fmt.Println("pending updates:", info.PendingUpdateCount)
		if info.LastErrorMessage != "" {
			fmt.Println("last error:", info.LastErrorMessage)
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}
