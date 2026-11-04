// Package notify sends notifications to users (e.g. APNs push for friend
// requests and session invites). Other services depend on the Notifier
// interface so they can be tested without sending anything.
package notify

import (
	"context"
	"log/slog"
)

type Message struct {
	Title string
	Body  string
}

type Notifier interface {
	Notify(ctx context.Context, userID int64, msg Message) error
}

// LogNotifier logs instead of sending; use it until APNs is wired up.
type LogNotifier struct{}

func (LogNotifier) Notify(ctx context.Context, userID int64, msg Message) error {
	slog.InfoContext(ctx, "notify", "user_id", userID, "title", msg.Title)
	return nil
}
