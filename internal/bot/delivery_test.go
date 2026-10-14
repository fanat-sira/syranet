package bot

import (
	"bytes"
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureTelegram struct{ messages []map[string]string }

func (c *captureTelegram) Do(r *http.Request) (*http.Response, error) {
	r.ParseForm()
	payload := map[string]string{}
	for k := range r.PostForm {
		payload[k] = r.PostForm.Get(k)
	}
	if strings.HasSuffix(r.URL.Path, "sendMessage") {
		c.messages = append(c.messages, payload)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true,"result":{"id":99,"is_bot":true,"first_name":"Synthetic","username":"fixture_bot"}}`)), Header: make(http.Header)}, nil
}
func TestGroupRequestCannotIssueCredential(t *testing.T) {
	capture := &captureTelegram{}
	api, e := tgbotapi.NewBotAPIWithClient("1:synthetic_fixture", tgbotapi.APIEndpoint, capture)
	if e != nil {
		t.Fatal(e)
	}
	b := &UserBot{api: api}
	m := &tgbotapi.Message{Text: "/getkey", From: &tgbotapi.User{ID: 1}, Chat: &tgbotapi.Chat{ID: -123, Type: "group"}}
	if e = b.handleMessage(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	if len(capture.messages) != 1 || strings.Contains(capture.messages[0]["text"], "hysteria2://") {
		t.Fatal("group request reached credential path")
	}
}
func TestSecretMessagesAreProtected(t *testing.T) {
	capture := &captureTelegram{}
	api, e := tgbotapi.NewBotAPIWithClient("1:synthetic_fixture", tgbotapi.APIEndpoint, capture)
	if e != nil {
		t.Fatal(e)
	}
	b := &UserBot{api: api}
	if e = b.sendSecret(1, "synthetic credential message"); e != nil {
		t.Fatal(e)
	}
	if len(capture.messages) != 1 || capture.messages[0]["protect_content"] != "true" || capture.messages[0]["chat_id"] != "1" {
		t.Fatal("credential delivery missing private message protection")
	}
}
