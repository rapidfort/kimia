package logger

import (
	"fmt"
	"log"
	"net/url"
	"os"
)

var (
	logLevel = "info"
	logDebug *log.Logger
	logInfo  *log.Logger
	logWarn  *log.Logger
	logError *log.Logger
	logFatal *log.Logger
)

func Setup(verbosity string, timestamp bool) {
	if verbosity != "" {
		logLevel = verbosity
	}

	prefix := ""
	if timestamp {
		prefix = "2006-01-02 15:04:05 "
	}

	logDebug = log.New(os.Stdout, prefix+"[DEBUG] ", 0)
	logInfo = log.New(os.Stdout, prefix+"[INFO] ", 0)
	logWarn = log.New(os.Stderr, prefix+"[WARN] ", 0)
	logError = log.New(os.Stderr, prefix+"[ERROR] ", 0)
	logFatal = log.New(os.Stderr, prefix+"[FATAL] ", 0)
}

func Debug(format string, args ...interface{}) {
	if logDebug == nil {
		return
	}
	if logLevel == "debug" {
		logDebug.Printf(format, args...)
	}
}

func Info(format string, args ...interface{}) {
	if logInfo == nil {
		fmt.Printf("[INFO] "+format+"\n", args...)
		return
	}
	if logLevel == "debug" || logLevel == "info" {
		logInfo.Printf(format, args...)
	}
}

func Warning(format string, args ...interface{}) {
	if logWarn == nil {
		fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
		return
	}
	if logLevel != "error" && logLevel != "fatal" {
		logWarn.Printf(format, args...)
	}
}

func Error(format string, args ...interface{}) {
	if logError == nil {
		fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
		return
	}
	logError.Printf(format, args...)
}

func Fatal(format string, args ...interface{}) {
	if logFatal == nil {
		fmt.Fprintf(os.Stderr, "[FATAL] "+format+"\n", args...)
		os.Exit(1)
	}
	logFatal.Printf(format, args...)
	os.Exit(1)
}

// SanitizeGitURL removes credentials from Git URLs for safe logging
// Preserves username but redacts password/token
func SanitizeGitURL(gitURL string) string {
	u, err := url.Parse(gitURL)
	if err != nil {
		// Not a valid URL, return as-is (might be SSH or local path)
		return gitURL
	}

	// If there's user info (credentials), redact the password but keep username
	if u.User != nil {
		username := u.User.Username()
		if _, hasPassword := u.User.Password(); hasPassword {
			// Manually reconstruct URL to avoid encoding **REDACTED**
			scheme := u.Scheme
			host := u.Host
			path := u.Path
			fragment := ""
			if u.Fragment != "" {
				fragment = "#" + u.Fragment
			}
			query := ""
			if u.RawQuery != "" {
				query = "?" + u.RawQuery
			}

			return fmt.Sprintf("%s://%s:**REDACTED**@%s%s%s%s", 
				scheme, username, host, path, query, fragment)
		}
	}

	return u.String()
}
