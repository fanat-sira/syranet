package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/models"
	"github.com/fanat-sira/syranet/internal/service"
)

type UserBot struct {
	api           *tgbotapi.BotAPI
	cfg           config.Config
	store         *database.Store
	questionnaire *QuestionnaireManager
	keygen        *service.KeyGenerator
	adminNotifier AdminNotifier
	logger        *zap.Logger
	accessSync    AccessSyncFunc
}

func NewUserBot(cfg config.Config, store *database.Store, questionnaire *QuestionnaireManager, keygen *service.KeyGenerator, adminNotifier AdminNotifier, logger *zap.Logger) (*UserBot, error) {
	api, err := newAPI(cfg.Bots.UserToken)
	if err != nil {
		return nil, errors.New("user bot initialization failed; check USER_BOT_TOKEN and Telegram connectivity")
	}
	api.Debug = false

	return &UserBot{
		api:           api,
		cfg:           cfg,
		store:         store,
		questionnaire: questionnaire,
		keygen:        keygen,
		adminNotifier: adminNotifier,
		logger:        logger.Named("user-bot"),
	}, nil
}

func (b *UserBot) Run(ctx context.Context) error {
	updateCfg := tgbotapi.NewUpdate(0)
	updateCfg.Timeout = b.cfg.Bots.PollingTimeout
	updates := b.api.GetUpdatesChan(updateCfg)
	defer b.api.StopReceivingUpdates()

	b.logger.Info("user bot started", zap.String("bot_username", b.api.Self.UserName))

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil || update.Message.From == nil {
				continue
			}
			if err := b.handleMessage(ctx, update.Message); err != nil {
				b.logger.Error("handle user message", zap.String("kind", "request failed"))
				_ = b.sendText(update.Message.Chat.ID, "Произошла ошибка. Попробуй ещё раз чуть позже.")
			}
		}
	}
}

func (b *UserBot) handleMessage(ctx context.Context, message *tgbotapi.Message) error {
	if !private(message) {
		if message != nil && message.Chat != nil {
			return b.sendText(message.Chat.ID, "Используй личный диалог с ботом.")
		}
		return nil
	}
	if len(message.Text) > 512 {
		return b.sendText(message.Chat.ID, "Сообщение слишком длинное.")
	}

	if message.IsCommand() {
		switch message.Command() {
		case "servers":
			return b.handleServers(message)
		case "select":
			return b.handleSelect(ctx, message)
		case "start":
			return b.handleStart(ctx, message)
		case "getkey":
			return b.handleGetKey(ctx, message)
		case "getsocks":
			return b.handleGetSocks(ctx, message)
		case "status":
			return b.handleStatus(ctx, message)
		case "extend":
			return b.handleExtendRequest(ctx, message)
		default:
			return b.sendText(message.Chat.ID, "Доступные команды: /start, /getkey, /getsocks, /status, /extend")
		}
	}

	if b.questionnaire.HasSession(message.From.ID) {
		return b.handleQuestionnaireReply(ctx, message)
	}

	return b.sendText(message.Chat.ID, "Используй /start для регистрации или одну из команд: /getkey, /getsocks, /status, /extend")
}

func (b *UserBot) handleStart(ctx context.Context, message *tgbotapi.Message) error {
	profile := database.TelegramProfile{TelegramID: message.From.ID, TelegramUsername: message.From.UserName}

	hy2User, hy2Pass, err := b.keygen.NewHysteriaCredentials()
	if err != nil {
		return err
	}
	socksUser, socksPass, err := b.keygen.NewSocksCredentials()
	if err != nil {
		return err
	}

	user, err := b.store.UpsertTelegramUser(ctx, profile, hy2User, hy2Pass, socksUser, socksPass)
	if err != nil {
		return err
	}

	if user.HasCompletedQuestionnaire() || user.Status != models.UserStatusPending {
		return b.sendText(message.Chat.ID, b.statusSummary(user, true))
	}

	prompt := b.questionnaire.Start(user)
	return b.sendText(message.Chat.ID, prompt)
}

func (b *UserBot) handleQuestionnaireReply(ctx context.Context, message *tgbotapi.Message) error {
	prompt, completed, answers, err := b.questionnaire.HandleResponse(message.From.ID, message.Text)
	if err != nil {
		return b.sendText(message.Chat.ID, "Ответ не должен быть пустым. Попробуй ещё раз.")
	}
	if !completed {
		return b.sendText(message.Chat.ID, prompt)
	}

	if err := b.store.UpdateQuestionnaire(ctx, message.From.ID, answers.ReferralSource, answers.IntendedUse, answers.AcceptedTerms); err != nil {
		return err
	}

	user, err := b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		return err
	}

	if b.adminNotifier != nil {
		text := fmt.Sprintf("New access request: user %d. /approve %d or /reject %d <reason>", user.ID, user.ID, user.ID)
		if err := b.adminNotifier.NotifyAdmins(ctx, text); err != nil {
			b.logger.Warn("notify admins about pending user", zap.String("kind", "operation failed"), zap.Int64("user_id", user.ID))
		}
	}

	return b.sendText(message.Chat.ID, prompt)
}

func (b *UserBot) handleGetKey(ctx context.Context, message *tgbotapi.Message) error {
	user, err := b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Сначала зарегистрируйся через /start")
		}
		return err
	}
	if !user.HasCompletedQuestionnaire() && user.Status == models.UserStatusPending {
		prompt := b.questionnaire.Start(user)
		return b.sendText(message.Chat.ID, prompt)
	}
	if !user.IsSubscriptionActive(time.Now()) {
		return b.sendText(message.Chat.ID, b.statusSummary(user, false))
	}
	n, ok := b.cfg.Node(user.NodeID)
	if !ok || !n.Entry {
		return b.sendText(message.Chat.ID, "Выбранный сервер недоступен. Используй /servers.")
	}
	if b.accessSync == nil || b.accessSync(ctx) != nil {
		return b.sendText(message.Chat.ID, "Доступ ещё применяется на сервере. Попробуй позже; администратор видит ошибку.")
	}
	user, err = b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		return err
	}
	if !user.IsSubscriptionActive(time.Now()) {
		return b.sendText(message.Chat.ID, b.statusSummary(user, false))
	}
	states, err := b.store.SyncStates(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		if !state.Healthy {
			return b.sendText(message.Chat.ID, "Состояние доступа изменилось; применение ещё выполняется.")
		}
	}
	n, ok = b.cfg.Node(user.NodeID)
	if !ok || !n.Entry {
		return b.sendText(message.Chat.ID, "Сервер недоступен.")
	}
	return b.sendSecret(message.Chat.ID, "Твой Hysteria2 ключ:\n"+service.BuildHysteriaLink(n, user.HY2Password))
}

func (b *UserBot) handleGetSocks(ctx context.Context, message *tgbotapi.Message) error {
	user, err := b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Сначала зарегистрируйся через /start")
		}
		return err
	}
	if !user.HasCompletedQuestionnaire() && user.Status == models.UserStatusPending {
		prompt := b.questionnaire.Start(user)
		return b.sendText(message.Chat.ID, prompt)
	}
	if !user.IsSubscriptionActive(time.Now()) {
		return b.sendText(message.Chat.ID, b.statusSummary(user, false))
	}
	n, ok := b.cfg.Node(user.NodeID)
	if !ok || !n.Socks.Enabled {
		return b.sendText(message.Chat.ID, "SOCKS5 выключен на выбранном сервере.")
	}
	if !n.Socks.AllowPublic {
		return b.sendText(message.Chat.ID, "SOCKS5 доступен только в приватной сети; публичная Telegram-ссылка не выдаётся.")
	}
	if b.accessSync == nil || b.accessSync(ctx) != nil {
		return b.sendText(message.Chat.ID, "Конфигурация сервера ещё не применена.")
	}
	user, err = b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		return err
	}
	if !user.IsSubscriptionActive(time.Now()) {
		return b.sendText(message.Chat.ID, b.statusSummary(user, false))
	}
	states, err := b.store.SyncStates(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		if !state.Healthy {
			return b.sendText(message.Chat.ID, "Изменение доступа ещё применяется.")
		}
	}
	n, ok = b.cfg.Node(user.NodeID)
	if !ok || !n.Socks.Enabled || !n.Socks.AllowPublic {
		return b.sendText(message.Chat.ID, "SOCKS5 недоступен.")
	}
	return b.sendSecret(message.Chat.ID, "SOCKS5 без шифрования; используй только в доверенной сети.\n"+service.BuildSocksLink(n, user.SocksUsername, user.SocksPassword))
}

func (b *UserBot) handleStatus(ctx context.Context, message *tgbotapi.Message) error {
	user, err := b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Профиль не найден. Используй /start")
		}
		return err
	}
	return b.sendText(message.Chat.ID, b.statusSummary(user, false))
}

func (b *UserBot) handleExtendRequest(ctx context.Context, message *tgbotapi.Message) error {
	user, err := b.store.GetUserByTelegramID(ctx, message.From.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Профиль не найден. Используй /start")
		}
		return err
	}
	if b.adminNotifier != nil {
		text := fmt.Sprintf("Extend request\nUser ID: %d\nStatus: %s", user.ID, user.Status)
		if err := b.adminNotifier.NotifyAdmins(ctx, text); err != nil {
			b.logger.Warn("notify admins about extend request", zap.String("kind", "operation failed"), zap.Int64("user_id", user.ID))
		}
	}
	return b.sendText(message.Chat.ID, "Запрос на продление отправлен админу.")
}

func (b *UserBot) sendText(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("/getkey"), tgbotapi.NewKeyboardButton("/status")), tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("/servers"), tgbotapi.NewKeyboardButton("/extend")))
	_, err := b.api.Send(msg)
	return err
}

func (b *UserBot) statusSummary(user *models.User, includeGreeting bool) string {
	builder := strings.Builder{}
	if includeGreeting {
		builder.WriteString("Привет. ")
	}
	builder.WriteString(fmt.Sprintf("Статус: %s\n", user.Status))
	if !user.HasCompletedQuestionnaire() {
		builder.WriteString("Анкета ещё не заполнена. Используй /start, чтобы продолжить.\n")
		return builder.String()
	}
	if user.ExpiresAt != nil {
		builder.WriteString(fmt.Sprintf("Подписка до: %s\n", user.ExpiresAt.Format("2006-01-02 15:04 MST")))
		builder.WriteString(fmt.Sprintf("Осталось дней: %d\n", user.DaysUntilExpiry(time.Now())))
	} else {
		builder.WriteString("Подписка ещё не назначена.\n")
	}
	if user.Status == models.UserStatusPending {
		builder.WriteString("Заявка ожидает одобрения администратора.")
	}
	if user.Status == models.UserStatusRejected {
		builder.WriteString("Заявка отклонена. Свяжись с администратором при необходимости.")
	}
	if user.Status == models.UserStatusSuspended {
		builder.WriteString("Доступ временно приостановлен.")
	}
	if user.Status == models.UserStatusRevoked {
		builder.WriteString("Ключ отозван. Обратись к администратору.")
	}
	if user.Status == models.UserStatusExpired {
		builder.WriteString("Подписка истекла. Используй /extend для запроса продления.")
	}
	if user.IsSubscriptionActive(time.Now()) {
		builder.WriteString("Доступ активен. Используй /getkey или /getsocks для получения конфигов.")
	}
	return builder.String()
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (b *UserBot) SetSync(fn AccessSyncFunc) { b.accessSync = fn }
func (b *UserBot) sendSecret(id int64, text string) error {
	_, e := b.api.MakeRequest("sendMessage", tgbotapi.Params{"chat_id": fmt.Sprint(id), "text": text, "protect_content": "true"})
	return e
}
func (b *UserBot) handleServers(m *tgbotapi.Message) error {
	var lines []string
	for _, n := range b.cfg.Entries() {
		lines = append(lines, n.Name+" — /select "+n.ID)
	}
	return b.sendText(m.Chat.ID, strings.Join(lines, "\n"))
}
func (b *UserBot) handleSelect(ctx context.Context, m *tgbotapi.Message) error {
	n, ok := b.cfg.Node(strings.TrimSpace(m.CommandArguments()))
	if !ok || !n.Entry {
		return b.sendText(m.Chat.ID, "Используй /select <server_id> из списка /servers.")
	}
	if e := b.store.SelectNode(ctx, m.From.ID, n.ID); e != nil {
		return e
	}
	if b.accessSync == nil || b.accessSync(ctx) != nil {
		return b.sendText(m.Chat.ID, "Сервер выбран; применение доступа ожидает восстановления связи.")
	}
	return b.sendText(m.Chat.ID, "Сервер выбран: "+n.Name)
}
