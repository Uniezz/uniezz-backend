package auth

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrUnknownUniversity    = errors.New("unknown university")
	ErrUniversityNotFound   = errors.New("university not found")
	ErrUniversityExists     = errors.New("university already exists")
	ErrDomainTaken          = errors.New("email domain belongs to another university")
	ErrFacultyInUse         = errors.New("faculty still has courses")
	ErrWrongAuthMethod      = errors.New("university does not support this authentication method")
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidReferralCode  = errors.New("invalid referral code")
	ErrReferralAlreadySet   = errors.New("account already has an inviter")
	ErrReferralWindowClosed = errors.New("account too old to apply an invitation")
	ErrEmailNotAllowed      = errors.New("email domain not allowed for this university")
	ErrStateNotFound        = errors.New("login state not found or expired")
	ErrInvalidCode          = errors.New("invalid or expired code")
	ErrNotStudent           = errors.New("not an active student")
	ErrRateLimited          = errors.New("too many attempts")
	ErrUpstream             = errors.New("identity provider unavailable")

	ErrInvalidIdentity = errors.New("invalid identity")

	ErrInvalidTokenString = errors.New("invalid token string")

	ErrSessionNotFound = errors.New("session not found")

	ErrForbidden = errors.New("role required")

	ErrUserNotFound = errors.New("user not found")
	ErrStaff        = errors.New("user has a staff role")
	ErrBanned       = errors.New("account banned")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrInvalidIdentity, Status: http.StatusInternalServerError, Code: "internal_error"},
	{Err: ErrUnknownUniversity, Status: http.StatusBadRequest, Code: "unknown_university"},
	{Err: ErrUniversityNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrUniversityExists, Status: http.StatusConflict, Code: "university_exists"},
	{Err: ErrDomainTaken, Status: http.StatusConflict, Code: "domain_taken"},
	{Err: ErrFacultyInUse, Status: http.StatusConflict, Code: "faculty_in_use"},
	{Err: ErrWrongAuthMethod, Status: http.StatusBadRequest, Code: "wrong_auth_method"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrInvalidReferralCode, Status: http.StatusBadRequest, Code: "invalid_referral_code"},
	{Err: ErrReferralAlreadySet, Status: http.StatusConflict, Code: "referral_already_set"},
	{Err: ErrReferralWindowClosed, Status: http.StatusForbidden, Code: "referral_window_closed"},
	{Err: ErrEmailNotAllowed, Status: http.StatusBadRequest, Code: "email_not_allowed"},
	{Err: ErrStateNotFound, Status: http.StatusBadRequest, Code: "login_expired"},
	{Err: ErrInvalidCode, Status: http.StatusUnauthorized, Code: "invalid_code"},
	{Err: ErrInvalidTokenString, Status: http.StatusUnauthorized, Code: "unauthorized"},
	{Err: ErrSessionNotFound, Status: http.StatusUnauthorized, Code: "unauthorized"},
	{Err: ErrNotStudent, Status: http.StatusForbidden, Code: "not_student"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: ErrBanned, Status: http.StatusForbidden, Code: "banned"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: ErrUpstream, Status: http.StatusBadGateway, Code: "provider_unavailable"},
}
