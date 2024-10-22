package constella

import "os"

var RELAY = getEnv("RELAY", "https://example.com")

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
