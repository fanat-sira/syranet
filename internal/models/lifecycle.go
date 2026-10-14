package models

import (
	"errors"
	"time"
)

// Transition is the subscription state machine. changed=false makes repeats no-ops.
func (u *User) Transition(action string, days int, now time.Time) (bool, error) {
	if (action == "approve" || action == "extend") && (days < 1 || days > 3650) {
		return false, errors.New("days must be between 1 and 3650")
	}
	switch action {
	case "approve":
		if u.Status == UserStatusActive {
			return false, nil
		}
		if u.Status != UserStatusPending && u.Status != UserStatusRejected {
			return false, errors.New("approve requires pending or rejected; use extend for expired access")
		}
		if !u.HasCompletedQuestionnaire() {
			return false, errors.New("questionnaire incomplete")
		}
		until := now.AddDate(0, 0, days)
		u.ExpiresAt = &until
		u.Status = UserStatusActive
	case "extend":
		if u.Status != UserStatusActive && u.Status != UserStatusExpired && u.Status != UserStatusSuspended {
			return false, errors.New("extend requires approved access")
		}
		start := now
		if u.ExpiresAt != nil && u.ExpiresAt.After(now) {
			start = *u.ExpiresAt
		}
		until := start.AddDate(0, 0, days)
		u.ExpiresAt = &until
		if u.Status == UserStatusExpired {
			u.Status = UserStatusActive
		}
	case "activate":
		if u.IsSubscriptionActive(now) {
			return false, nil
		}
		if u.Status != UserStatusSuspended || u.ExpiresAt == nil || !u.ExpiresAt.After(now) {
			return false, errors.New("activation requires suspended access with an unexpired subscription")
		}
		u.Status = UserStatusActive
	case "suspend":
		if u.Status == UserStatusSuspended {
			return false, nil
		}
		if u.Status != UserStatusActive {
			return false, errors.New("suspend requires active access")
		}
		u.Status = UserStatusSuspended
	case "reject":
		if u.Status == UserStatusRejected {
			return false, nil
		}
		if u.Status != UserStatusPending {
			return false, errors.New("reject requires pending access")
		}
		u.Status = UserStatusRejected
	case "revoke":
		if u.Status == UserStatusRevoked {
			return false, nil
		}
		u.Status = UserStatusRevoked
	case "rotate":
		if u.Status == UserStatusRevoked {
			u.Status = UserStatusSuspended
		}
	default:
		return false, errors.New("unknown action")
	}
	return true, nil
}
