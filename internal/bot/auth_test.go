package bot

import (
	"github.com/fanat-sira/syranet/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"testing"
)

func TestPrivateAuthorization(t *testing.T) {
	c := config.Config{Bots: config.BotsConfig{AdminIDs: []int64{1}}}
	for _, v := range []struct {
		typ        string
		chat, user int64
		want       bool
	}{{"private", 1, 1, true}, {"group", 1, 1, false}, {"supergroup", -1, 1, false}, {"private", 2, 2, false}, {"private", 2, 1, false}} {
		m := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: v.chat, Type: v.typ}, From: &tgbotapi.User{ID: v.user}}
		if authorized(c, m) != v.want {
			t.Fatal(v)
		}
	}
	if authorized(c, nil) || private(&tgbotapi.Message{}) {
		t.Fatal("nil request authorized")
	}
}
func TestQuestionnairePreservesAnswers(t *testing.T) {
	m := NewQuestionnaireManager()
	if !acceptsTerms("yes") || acceptsTerms("no") {
		t.Fatal("terms parsing")
	}
	if _, done, _, e := m.HandleResponse(999, "yes"); done || e != nil {
		t.Fatal("unknown session completed")
	}
}
