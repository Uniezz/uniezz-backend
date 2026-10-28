package auth

type TrustBadge string

const (
	TrustBadgeVerified  TrustBadge = "verified"
	TrustBadgeDirectory TrustBadge = "directory"
	TrustBadgeDomain    TrustBadge = "domain"
)

func trustBadgeOf(u *User) TrustBadge {
	if u.UsosUserID != nil {
		return TrustBadgeVerified
	}
	return TrustBadgeDomain
}
