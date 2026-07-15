package notification

import "time"

// CalcBackoff экспортирует calcBackoff для тестирования.
func CalcBackoff(base time.Duration, attempt int) time.Duration {
	return calcBackoff(base, attempt)
}

// BackoffCap экспортирует backoffCap для тестирования.
const BackoffCap = backoffCap
