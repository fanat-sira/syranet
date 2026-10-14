package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/fanat-sira/syranet/internal/config"
	"github.com/fanat-sira/syranet/internal/database"
	"github.com/fanat-sira/syranet/internal/models"
)

const adminUsersPageSize = 10

type AccessSyncFunc func(ctx context.Context) error

type AdminBot struct {
	api        *tgbotapi.BotAPI
	cfg        config.Config
	store      *database.Store
	accessSync AccessSyncFunc
	logger     *zap.Logger
}

func NewAdminBot(cfg config.Config, store *database.Store, accessSync AccessSyncFunc, logger *zap.Logger) (*AdminBot, error) {
	api, err := newAPI(cfg.Bots.AdminToken)
	if err != nil {
		return nil, errors.New("admin bot initialization failed; check ADMIN_BOT_TOKEN and Telegram connectivity")
	}
	api.Debug = false

	return &AdminBot{
		api:        api,
		cfg:        cfg,
		store:      store,
		accessSync: accessSync,
		logger:     logger.Named("admin-bot"),
	}, nil
}

func (b *AdminBot) Run(ctx context.Context) error {
	updateCfg := tgbotapi.NewUpdate(0)
	updateCfg.Timeout = b.cfg.Bots.PollingTimeout
	updates := b.api.GetUpdatesChan(updateCfg)
	defer b.api.StopReceivingUpdates()

	b.logger.Info("admin bot started", zap.String("bot_username", b.api.Self.UserName))

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
			if !authorized(b.cfg, update.Message) {
				_ = b.sendText(update.Message.Chat.ID, "Доступ запрещён.")
				continue
			}
			if err := b.handleMessage(ctx, update.Message); err != nil {
				b.logger.Error("handle admin message", zap.String("kind", "request failed"))
				_ = b.sendText(update.Message.Chat.ID, "Не удалось выполнить команду.")
			}
		}
	}
}

func (b *AdminBot) NotifyAdmins(ctx context.Context, text string) error {
	var sendErr error
	for _, adminID := range b.cfg.Bots.AdminIDs {
		if err := b.sendText(adminID, text); err != nil {
			b.logger.Warn("notify admin", zap.Int64("admin_id", adminID), zap.String("kind", "operation failed"))
			sendErr = err
		}
	}
	return sendErr
}

func (b *AdminBot) handleMessage(ctx context.Context, message *tgbotapi.Message) error {
	if !authorized(b.cfg, message) {
		return errors.New("unauthorized private admin request")
	}
	if len(message.Text) > 512 {
		return b.sendText(message.Chat.ID, "Сообщение слишком длинное.")
	}
	ctx = database.WithRequest(ctx, fmt.Sprintf("%d:%d", message.Chat.ID, message.MessageID))

	if !message.IsCommand() {
		return b.sendText(message.Chat.ID, b.helpText())
	}

	switch message.Command() {
	case "nodes":
		return b.handleNodes(ctx, message)
	case "search":
		users, e := b.store.Search(ctx, strings.TrimSpace(message.CommandArguments()))
		if e != nil {
			return e
		}
		var lines []string
		for _, u := range users {
			lines = append(lines, formatUserInfo(&u))
		}
		if len(lines) == 0 {
			lines = append(lines, "Не найдено.")
		}
		return b.sendText(message.Chat.ID, strings.Join(lines, "\n"))
	case "revoke", "rotate":
		id, e := parseSingleInt64Arg(message.CommandArguments())
		if e != nil || id <= 0 {
			return b.sendText(message.Chat.ID, "Использование: /"+message.Command()+" <user_id>")
		}
		if e = b.store.Action(ctx, message.From.ID, id, message.Command(), 0, ""); e != nil {
			return b.sendText(message.Chat.ID, "Операция недоступна для текущего состояния пользователя.")
		}
		return b.sendSyncAwareReply(ctx, message.Chat.ID, "Состояние credentials изменено.")
	case "pending":
		return b.handlePending(ctx, message)
	case "approve":
		return b.handleApprove(ctx, message)
	case "reject":
		return b.handleReject(ctx, message)
	case "users":
		return b.handleUsers(ctx, message)
	case "extend":
		return b.handleExtend(ctx, message)
	case "suspend":
		return b.handleSuspend(ctx, message)
	case "activate":
		return b.handleActivate(ctx, message)
	case "info":
		return b.handleInfo(ctx, message)
	case "stats":
		return b.handleStats(ctx, message)
	case "start", "help":
		return b.sendText(message.Chat.ID, b.helpText())
	default:
		return b.sendText(message.Chat.ID, b.helpText())
	}
}

func (b *AdminBot) handlePending(ctx context.Context, message *tgbotapi.Message) error {
	users, err := b.store.ListPendingUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return b.sendText(message.Chat.ID, "Нет пользователей в статусе pending.")
	}

	lines := []string{"Pending users:"}
	for _, user := range users {
		lines = append(lines, fmt.Sprintf("User %d: /approve %d, /reject %d <reason>", user.ID, user.ID, user.ID))
	}
	return b.sendText(message.Chat.ID, strings.Join(lines, "\n"))
}

func (b *AdminBot) handleApprove(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 1 {
		return b.sendText(message.Chat.ID, "Использование: /approve <user_id>")
	}
	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return b.sendText(message.Chat.ID, "Некорректный user_id")
	}
	if err := b.store.ApproveUser(ctx, message.From.ID, userID, b.cfg.VPN.SubscriptionDays); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден или не может быть одобрен.")
		}
		return err
	}
	return b.sendSyncAwareReply(ctx, message.Chat.ID, fmt.Sprintf("Одобрение пользователя %d сохранено; действующий срок не сбрасывается.", userID))
}

func (b *AdminBot) handleReject(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) < 2 {
		return b.sendText(message.Chat.ID, "Использование: /reject <user_id> <reason>")
	}
	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return b.sendText(message.Chat.ID, "Некорректный user_id")
	}
	reason := strings.Join(args[1:], " ")
	if err := b.store.RejectUser(ctx, message.From.ID, userID, reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден.")
		}
		return err
	}
	return b.sendSyncAwareReply(ctx, message.Chat.ID, fmt.Sprintf("Пользователь %d отклонён.", userID))
}

func (b *AdminBot) handleUsers(ctx context.Context, message *tgbotapi.Message) error {
	page := 1
	if arg := strings.TrimSpace(message.CommandArguments()); arg != "" {
		parsed, err := strconv.Atoi(arg)
		if err == nil && parsed > 0 {
			page = parsed
		}
	}
	offset := (page - 1) * adminUsersPageSize
	users, total, err := b.store.ListUsersByStatuses(ctx, []models.UserStatus{models.UserStatusActive, models.UserStatusRevoked, models.UserStatusSuspended, models.UserStatusExpired, models.UserStatusPending, models.UserStatusRejected}, adminUsersPageSize, offset)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return b.sendText(message.Chat.ID, "Список пользователей пуст для выбранной страницы.")
	}

	lines := []string{fmt.Sprintf("Users page %d (total=%d):", page, total)}
	for _, user := range users {
		expiresAt := "-"
		if user.ExpiresAt != nil {
			expiresAt = user.ExpiresAt.Format("2006-01-02")
		}
		lines = append(lines, fmt.Sprintf("ID=%d | @%s | status=%s | expires=%s", user.ID, emptyFallback(user.TelegramUsername, "unknown"), user.Status, expiresAt))
	}
	return b.sendText(message.Chat.ID, strings.Join(lines, "\n"))
}

func (b *AdminBot) handleExtend(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 2 {
		return b.sendText(message.Chat.ID, "Использование: /extend <user_id> <days>")
	}
	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return b.sendText(message.Chat.ID, "Некорректный user_id")
	}
	days, err := strconv.Atoi(args[1])
	if err != nil || days <= 0 {
		return b.sendText(message.Chat.ID, "Некорректное количество дней")
	}
	if err := b.store.ExtendUser(ctx, message.From.ID, userID, days, "manual extension"); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден.")
		}
		return err
	}
	return b.sendSyncAwareReply(ctx, message.Chat.ID, fmt.Sprintf("Подписка пользователя %d продлена на %d дней.", userID, days))
}

func (b *AdminBot) handleSuspend(ctx context.Context, message *tgbotapi.Message) error {
	userID, err := parseSingleInt64Arg(message.CommandArguments())
	if err != nil {
		return b.sendText(message.Chat.ID, "Использование: /suspend <user_id>")
	}
	if err := b.store.SuspendUser(ctx, message.From.ID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден.")
		}
		return err
	}
	return b.sendSyncAwareReply(ctx, message.Chat.ID, fmt.Sprintf("Пользователь %d приостановлен.", userID))
}

func (b *AdminBot) handleActivate(ctx context.Context, message *tgbotapi.Message) error {
	userID, err := parseSingleInt64Arg(message.CommandArguments())
	if err != nil {
		return b.sendText(message.Chat.ID, "Использование: /activate <user_id>")
	}
	if err := b.store.ActivateUser(ctx, message.From.ID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден.")
		}
		return err
	}
	return b.sendSyncAwareReply(ctx, message.Chat.ID, fmt.Sprintf("Пользователь %d активирован.", userID))
}

func (b *AdminBot) handleInfo(ctx context.Context, message *tgbotapi.Message) error {
	userID, err := parseSingleInt64Arg(message.CommandArguments())
	if err != nil {
		return b.sendText(message.Chat.ID, "Использование: /info <user_id>")
	}
	user, err := b.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return b.sendText(message.Chat.ID, "Пользователь не найден.")
		}
		return err
	}
	return b.sendText(message.Chat.ID, formatReviewInfo(user))
}

func (b *AdminBot) handleStats(ctx context.Context, message *tgbotapi.Message) error {
	stats, err := b.store.GetStats(ctx)
	if err != nil {
		return err
	}
	text := fmt.Sprintf(
		"Stats:\nTotal: %d\nActive: %d\nPending: %d\nExpired: %d\nRevoked: %d\nRejected: %d\nSuspended: %d",
		stats.Total,
		stats.Active,
		stats.Pending,
		stats.Expired,
		stats.Revoked,
		stats.Rejected,
		stats.Suspended,
	)
	return b.sendText(message.Chat.ID, text)
}

func (b *AdminBot) sendSyncAwareReply(ctx context.Context, chatID int64, successText string) error {
	if b.accessSync == nil {
		return b.sendText(chatID, successText+"\nСинхронизация недоступна.")
	}
	if err := b.accessSync(ctx); err != nil {
		b.logger.Error("sync access after admin command", zap.String("kind", "operation failed"))
		return b.sendText(chatID, successText+"\nИзменение сохранено, но ещё не применено на сервере. Будет повторная попытка; проверь /nodes.")
	}
	return b.sendText(chatID, successText+"\nКонфигурация sing-box синхронизирована.")
}

func (b *AdminBot) sendText(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	return err
}

func (b *AdminBot) helpText() string {
	return strings.Join([]string{
		"Admin commands:",
		"/pending - список pending",
		fmt.Sprintf("/approve <user_id> - одобрить на %d дней", b.cfg.VPN.SubscriptionDays),
		"/reject <user_id> <reason> - отклонить",
		"/users [page] - активные/архивные пользователи",
		"/extend <user_id> <days> - продлить",
		"/suspend <user_id> - приостановить",
		"/activate <user_id> - активировать",
		"/info <user_id> - детали",
		"/stats - статистика",
		"/search <id|telegram_id|@username>",
		"/revoke <id> - отозвать доступ",
		"/rotate <id> - заменить credentials",
		"/nodes - применение конфигурации",
	}, "\n")
}

func parseSingleInt64Arg(raw string) (int64, error) {
	args := strings.Fields(raw)
	if len(args) != 1 {
		return 0, errors.New("invalid args")
	}
	return strconv.ParseInt(args[0], 10, 64)
}

func formatUserInfo(u *models.User) string {
	expiry := "not assigned"
	if u.ExpiresAt != nil {
		expiry = u.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("User %d | @%s\nStatus: %s\nExpires: %s\nEntry: %s", u.ID, u.TelegramUsername, u.Status, expiry, u.NodeID)
}
func (b *AdminBot) handleNodes(ctx context.Context, m *tgbotapi.Message) error {
	states, e := b.store.SyncStates(ctx)
	if e != nil {
		return e
	}
	lines := []string{"Node sync status:"}
	for _, s := range states {
		last := "never"
		if s.LastSuccess != nil {
			last = s.LastSuccess.UTC().Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("%s: healthy=%t desired=%s applied=%s last=%s %s", s.NodeID, s.Healthy, shortHash(s.DesiredHash), shortHash(s.AppliedHash), last, s.Error))
	}
	return b.sendText(m.Chat.ID, strings.Join(lines, "\n"))
}
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func formatReviewInfo(u *models.User) string {
	return formatUserInfo(u) + fmt.Sprintf("\nReferral: %s\nPurpose: %s\nAccepted terms: %t", bounded(u.ReferralSource), bounded(u.IntendedUse), u.AcceptedTerms)
}
func bounded(s string) string {
	r := []rune(s)
	if len(r) > 128 {
		return string(r[:128]) + "…"
	}
	return s
}
