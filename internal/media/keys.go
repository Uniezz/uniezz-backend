package media

import (
	"fmt"
	"strings"
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
