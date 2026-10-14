package bot

import (
	"github.com/fanat-sira/syranet/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func private(m *tgbotapi.Message) bool {
	return m != nil && m.From != nil && m.Chat != nil && m.Chat.IsPrivate() && m.Chat.ID == m.From.ID
}
func authorized(c config.Config, m *tgbotapi.Message) bool { return private(m) && c.IsAdmin(m.From.ID) }
