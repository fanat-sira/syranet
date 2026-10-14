package bot

import "context"

type AdminNotifier interface {
	NotifyAdmins(ctx context.Context, text string) error
}
