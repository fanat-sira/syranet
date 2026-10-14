package bot

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fanat-sira/syranet/internal/models"
)

type QuestionStep int

const (
	StepReferral QuestionStep = iota + 1
	StepIntendedUse
	StepTerms
	StepCompleted
)

type QuestionnaireAnswers struct {
	ReferralSource string
	IntendedUse    string
	AcceptedTerms  bool
}

type questionnaireSession struct {
	step    QuestionStep
	answers QuestionnaireAnswers
}

type QuestionnaireManager struct {
	mu       sync.RWMutex
	sessions map[int64]*questionnaireSession
}

func NewQuestionnaireManager() *QuestionnaireManager {
	return &QuestionnaireManager{
		sessions: make(map[int64]*questionnaireSession),
	}
}

func (m *QuestionnaireManager) Start(user *models.User) string {
	if user == nil {
		return stepPrompt(StepReferral)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	session := &questionnaireSession{answers: QuestionnaireAnswers{}}
	if user != nil {
		session.answers.ReferralSource = user.ReferralSource
		session.answers.IntendedUse = user.IntendedUse
		session.answers.AcceptedTerms = user.AcceptedTerms
	}

	session.step = determineStep(user)
	m.sessions[user.TelegramID] = session
	return stepPrompt(session.step)
}

func (m *QuestionnaireManager) HasSession(telegramID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[telegramID]
	return ok
}

func (m *QuestionnaireManager) HandleResponse(telegramID int64, text string) (string, bool, QuestionnaireAnswers, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[telegramID]
	if !ok {
		return "", false, QuestionnaireAnswers{}, nil
	}

	clean := strings.TrimSpace(text)
	if clean == "" {
		return stepPrompt(session.step), false, session.answers, fmt.Errorf("empty answer")
	}

	switch session.step {
	case StepReferral:
		session.answers.ReferralSource = clean
		session.step = StepIntendedUse
		return stepPrompt(session.step), false, session.answers, nil
	case StepIntendedUse:
		session.answers.IntendedUse = clean
		session.step = StepTerms
		return stepPrompt(session.step), false, session.answers, nil
	case StepTerms:
		if !acceptsTerms(clean) {
			return "Для продолжения нужно согласиться с правилами. Ответь: Да, согласен.", false, session.answers, nil
		}
		session.answers.AcceptedTerms = true
		answers := session.answers
		delete(m.sessions, telegramID)
		return "Анкета заполнена. Заявка отправлена на проверку админу.", true, answers, nil
	default:
		delete(m.sessions, telegramID)
		return "", true, session.answers, nil
	}
}

func determineStep(user *models.User) QuestionStep {
	if user == nil || user.ReferralSource == "" {
		return StepReferral
	}
	if user.IntendedUse == "" {
		return StepIntendedUse
	}
	if !user.AcceptedTerms {
		return StepTerms
	}
	return StepCompleted
}

func stepPrompt(step QuestionStep) string {
	switch step {
	case StepReferral:
		return "Привет. Перед выдачей доступа нужно пройти короткий опрос.\n\n1) Откуда ты узнал о сервисе?"
	case StepIntendedUse:
		return "2) Для чего планируешь использовать сервис?"
	case StepTerms:
		return "3) Подтверди согласие с правилами: сервис нельзя использовать в запрещённых целях.\n\nОтветь: Да, согласен"
	default:
		return "Анкета уже заполнена."
	}
}

func acceptsTerms(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch normalized {
	case "да", "да согласен", "согласен", "yes", "i agree", "agree", "ok":
		return true
	default:
		return false
	}
}
