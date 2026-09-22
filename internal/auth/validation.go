package auth

import "net/mail"

// ValidCharacterName is shared by every character creation and rename path.
func ValidCharacterName(name string) bool {
	if len(name) < 3 || len(name) > 30 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == ' ' || r == '\'' || r == '-') {
			return false
		}
	}
	return true
}

func ValidEmail(email string) bool {
	a, err := mail.ParseAddress(email)
	return err == nil && a.Address == email && len(email) <= 254
}
