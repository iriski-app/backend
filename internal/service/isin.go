package service

import "regexp"

var isinPattern = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

func ValidateISIN(isin string) error {
	if !isinPattern.MatchString(isin) {
		return ErrInvalidISIN
	}

	digits := make([]int, 0, 24)
	for i := 0; i < len(isin); i++ {
		char := isin[i]
		if char >= '0' && char <= '9' {
			digits = append(digits, int(char-'0'))
		} else {
			value := int(char-'A') + 10
			digits = append(digits, value/10, value%10)
		}
	}

	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		digit := digits[i]
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	if sum%10 != 0 {
		return ErrInvalidISIN
	}
	return nil
}
