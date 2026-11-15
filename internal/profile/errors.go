package profile

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrGenderImmutable       = errors.New("gender cannot be changed or cleared once set")
	ErrBirthDateImmutable    = errors.New("birth date cannot be changed or cleared once set")
	ErrDatingRequiresProfile = errors.New("dating requires a birth date and gender")
	ErrNotFound              = errors.New("not found")
	ErrInvalidInput          = errors.New("invalid input")
	ErrInvalidAvatar         = errors.New("avatar media is missing, not owned by the user or not uploaded")
	ErrInvalidBanner         = errors.New("banner media is missing, not owned by the user or not uploaded")
	ErrReauthRequired        = errors.New("sign in again to delete the account")
	ErrInvalidProfilePhoto   = errors.New("profile photo is missing, not owned by the user or not uploaded")
	ErrProfilePhotoLimit     = errors.New("profile photo limit reached")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrGenderImmutable, Status: http.StatusBadRequest, Code: "gender_immutable"},
	{Err: ErrBirthDateImmutable, Status: http.StatusBadRequest, Code: "birth_date_immutable"},
	{Err: ErrDatingRequiresProfile, Status: http.StatusBadRequest, Code: "dating_requires_profile"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrInvalidAvatar, Status: http.StatusBadRequest, Code: "invalid_avatar"},
	{Err: ErrInvalidBanner, Status: http.StatusBadRequest, Code: "invalid_banner"},
	{Err: ErrReauthRequired, Status: http.StatusForbidden, Code: "reauthentication_required"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
	{Err: ErrInvalidProfilePhoto, Status: http.StatusBadRequest, Code: "invalid_profile_photo"},
	{Err: ErrProfilePhotoLimit, Status: http.StatusBadRequest, Code: "profile_photo_limit"},
}
