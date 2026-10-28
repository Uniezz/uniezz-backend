package auth

import "testing"

func TestTrustBadgeOf(t *testing.T) {
	usosID := "12345"
	email := "jan@student.kul.pl"

	tests := []struct {
		name string
		user User
		want TrustBadge
	}{
		{name: "USOS login is verified", user: User{UniversityID: UniversityUMCS, UsosUserID: &usosID}, want: TrustBadgeVerified},
		{name: "USOS login with email is still verified", user: User{UniversityID: UniversityUMCS, UsosUserID: &usosID, Email: &email}, want: TrustBadgeVerified},
		{name: "email OTP login is domain", user: User{UniversityID: UniversityKUL, Email: &email}, want: TrustBadgeDomain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trustBadgeOf(&tt.user); got != tt.want {
				t.Errorf("trustBadgeOf() = %q, want %q", got, tt.want)
			}
		})
	}
}
