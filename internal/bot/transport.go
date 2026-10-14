package bot

import (
	"errors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"net/http"
	"time"
)

// Telegram's request URL contains the bot token: never propagate transport errors.
type safeHTTP struct{ client *http.Client }

func (s safeHTTP) Do(r *http.Request) (*http.Response, error) {
	res, e := s.client.Do(r)
	if e != nil {
		return nil, errors.New("Telegram transport unavailable")
	}
	return res, nil
}
func newAPI(token string) (*tgbotapi.BotAPI, error) {
	return tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, safeHTTP{&http.Client{Timeout: 70 * time.Second}})
}
