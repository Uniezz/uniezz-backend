package media

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func AvatarKey(userID, filename string) (string, error) {
	return objectKey("avatars", userID, filename)
}

func FeedKey(postID, filename string) (string, error) {
	return objectKey("feed", postID, filename)
}

func ChatKey(threadID, filename string) (string, error) {
	return objectKey("chat", threadID, filename)
}

func objectKey(category, id, filename string) (string, error) {
	if err := validateKeyPart("id", id); err != nil {
		return "", err
	}
	if err := validateKeyPart("filename", filename); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/%s", category, id, filename), nil
}

func validateKeyPart(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value ||
		value == "." || value == ".." ||
		strings.ContainsAny(value, `/\`) ||
		strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("invalid media %s", name)
	}
	return nil
}

func NewObjectKey(kind Kind, contentType string) (string, error) {
	if err := validateUpload(kind, contentType, 1); err != nil {
		return "", err
	}

	prefix := ""
	switch kind {
	case KindAvatar:
		prefix = "avatars"
	case KindFeed:
		prefix = "feed"
	case KindChat:
		prefix = "chat"
	}

	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	}

	return fmt.Sprintf("%s/%s%s", prefix, uuid.NewString(), extension), nil
}
