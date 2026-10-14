package models

import "time"

type UserStatus string

const (
	UserStatusPending   UserStatus = "pending"
	UserStatusApproved  UserStatus = "approved"
	UserStatusActive    UserStatus = "active"
	UserStatusExpired   UserStatus = "expired"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusRejected  UserStatus = "rejected"
)

type User struct {
	ID                 int64      `db:"id"`
	TelegramID         int64      `db:"telegram_id"`
	TelegramUsername   string     `db:"telegram_username"`
	FirstName          string     `db:"first_name"`
	LastName           string     `db:"last_name"`
	HY2Username        string     `db:"hy2_username"`
	HY2Password        string     `db:"hy2_password"`
	SocksUsername      string     `db:"socks_username"`
	SocksPassword      string     `db:"socks_password"`
	Status             UserStatus `db:"status"`
	CreatedAt          time.Time  `db:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at"`
	ApprovedAt         *time.Time `db:"approved_at"`
	ApprovedBy         *int64     `db:"approved_by"`
	ExpiresAt          *time.Time `db:"expires_at"`
	ReferralSource     string     `db:"referral_source"`
	IntendedUse        string     `db:"intended_use"`
	AcceptedTerms      bool       `db:"accepted_terms"`
	LastConnectedAt    *time.Time `db:"last_connected_at"`
	LastConnectedIP    string     `db:"last_connected_ip"`
	CurrentConnections int        `db:"current_connections"`
}

type AdminLog struct {
	ID           int64     `db:"id"`
	AdminID      int64     `db:"admin_id"`
	TargetUserID *int64    `db:"target_user_id"`
	Action       string    `db:"action"`
	Reason       string    `db:"reason"`
	CreatedAt    time.Time `db:"created_at"`
}

type UserStats struct {
	Total     int `db:"total"`
	Active    int `db:"active"`
	Pending   int `db:"pending"`
	Expired   int `db:"expired"`
	Approved  int `db:"approved"`
	Rejected  int `db:"rejected"`
	Suspended int `db:"suspended"`
}

func (u User) HasCompletedQuestionnaire() bool {
	return u.ReferralSource != "" && u.IntendedUse != "" && u.AcceptedTerms
}

func (u User) IsSubscriptionActive(now time.Time) bool {
	if u.Status != UserStatusApproved && u.Status != UserStatusActive {
		return false
	}
	if u.ExpiresAt == nil {
		return false
	}
	return u.ExpiresAt.After(now)
}

func (u User) DaysUntilExpiry(now time.Time) int {
	if u.ExpiresAt == nil {
		return 0
	}
	duration := u.ExpiresAt.Sub(now)
	if duration <= 0 {
		return 0
	}
	return int(duration.Hours()/24) + 1
}
