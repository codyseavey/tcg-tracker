package services

import (
	"log"
	"os"
	"strings"
)

var translationDebugEnabled = false

func init() {
	// Enable debug logging if TRANSLATION_DEBUG=1 or TRANSLATION_DEBUG=true
	if v := os.Getenv("TRANSLATION_DEBUG"); v != "" {
		v = strings.ToLower(v)
		translationDebugEnabled = v == "1" || v == "true" || v == "yes"
		if translationDebugEnabled {
			log.Println("[TRANSLATION] Debug logging: ENABLED")
		}
	}
}

// debugLog logs only when TRANSLATION_DEBUG is enabled.
// Use this for verbose per-request details, OCR text, cache hits/misses, etc.
func debugLog(format string, args ...interface{}) {
	if translationDebugEnabled {
		log.Printf("[TRANSLATION DEBUG] "+format, args...)
	}
}

// infoLog always logs important translation events.
// Use this for translation fallback triggers, API errors, cache stats, etc.
func infoLog(format string, args ...interface{}) {
	log.Printf("[TRANSLATION] "+format, args...)
}
