package anonymizer

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Anonymizer handles data masking and anonymization
type Anonymizer struct {
	domains []string
}

// NewAnonymizer creates a new anonymizer instance
func NewAnonymizer() *Anonymizer {
	return &Anonymizer{
		domains: []string{"example.com", "test.com", "sample.org"},
	}
}

// AnonymizeEmail masks an email address
func (a *Anonymizer) AnonymizeEmail(email string) string {
	if email == "" {
		return ""
	}

	// Extract username and domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "anonymous@example.com"
	}

	// Use first character + random string
	username := parts[0]
	if len(username) > 0 {
		masked := string(username[0]) + strings.Repeat("*", min(len(username)-1, 5))
		domain := a.domains[randomInt(len(a.domains))]
		return fmt.Sprintf("%s@%s", masked, domain)
	}

	return "anonymous@example.com"
}

// AnonymizePhone masks a phone number
func (a *Anonymizer) AnonymizePhone(phone string) string {
	if phone == "" {
		return ""
	}

	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(phone, "")

	if len(digits) == 0 {
		return "+1-555-0100"
	}

	// Keep first 2 digits (country code), mask rest
	if len(digits) >= 10 {
		return fmt.Sprintf("+%s-555-%04d", digits[:2], randomInt(10000))
	}

	return "+1-555-0100"
}

// AnonymizeName masks a person's name
func (a *Anonymizer) AnonymizeName(name string) string {
	if name == "" {
		return ""
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "Anonymous User"
	}

	// Keep first character of each part
	var masked []string
	for _, part := range parts {
		if len(part) > 0 {
			masked = append(masked, string(part[0])+"***")
		}
	}

	return strings.Join(masked, " ")
}

// AnonymizePassword generates a bcrypt hash of a default password
func (a *Anonymizer) AnonymizePassword() string {
	// Use a standard anonymized password
	hash, err := bcrypt.GenerateFromPassword([]byte("changeme123"), bcrypt.DefaultCost)
	if err != nil {
		// Fallback to a pre-computed hash
		return "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy" // hash of "changeme123"
	}
	return string(hash)
}

// AnonymizeSSN masks a social security number
func (a *Anonymizer) AnonymizeSSN(ssn string) string {
	if ssn == "" {
		return ""
	}

	// Generate fake SSN: XXX-XX-1234
	return fmt.Sprintf("***-**-%04d", randomInt(10000))
}

// AnonymizeCreditCard masks a credit card number
func (a *Anonymizer) AnonymizeCreditCard(cc string) string {
	if cc == "" {
		return ""
	}

	// Remove all non-digit characters
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(cc, "")

	if len(digits) >= 4 {
		// Keep last 4 digits
		lastFour := digits[len(digits)-4:]
		return fmt.Sprintf("****-****-****-%s", lastFour)
	}

	return "****-****-****-0000"
}

// AnonymizeAddress masks an address
func (a *Anonymizer) AnonymizeAddress(address string) string {
	if address == "" {
		return ""
	}

	// Generate generic address
	return fmt.Sprintf("%d Anonymous Street, Privacy City, XX 00000", randomInt(9999)+1)
}

// AnonymizeValue attempts to anonymize a value based on field name and type
func (a *Anonymizer) AnonymizeValue(fieldName string, value interface{}) interface{} {
	if value == nil {
		return nil
	}

	strValue, ok := value.(string)
	if !ok {
		return value // Don't anonymize non-string values
	}

	fieldLower := strings.ToLower(fieldName)

	// Match common field patterns
	switch {
	case containsAny(fieldLower, []string{"email", "mail"}):
		return a.AnonymizeEmail(strValue)
	case containsAny(fieldLower, []string{"phone", "mobile", "tel"}):
		return a.AnonymizePhone(strValue)
	case containsAny(fieldLower, []string{"password", "passwd", "pwd"}):
		return a.AnonymizePassword()
	case containsAny(fieldLower, []string{"name", "firstname", "lastname", "fullname"}):
		return a.AnonymizeName(strValue)
	case containsAny(fieldLower, []string{"ssn", "social"}):
		return a.AnonymizeSSN(strValue)
	case containsAny(fieldLower, []string{"credit", "card", "cc"}):
		return a.AnonymizeCreditCard(strValue)
	case containsAny(fieldLower, []string{"address", "street", "addr"}):
		return a.AnonymizeAddress(strValue)
	default:
		return value
	}
}

// Helper functions

func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func containsAny(str string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(str, substr) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
