package middleware

import (
	"net/http"
	"runtime/debug"
)

// LogFunc is a function type for logging panic information
type LogFunc func(format string, args ...interface{})

// PanicMetricFunc is called when a panic is recovered to record metrics
type PanicMetricFunc func()

// Recovery returns middleware that recovers from panics and logs the error with stack trace
func Recovery(logFunc LogFunc) func(http.Handler) http.Handler {
	return RecoveryWithMetrics(logFunc, nil)
}

// RecoveryWithMetrics returns middleware that recovers from panics, logs errors, and records metrics
func RecoveryWithMetrics(logFunc LogFunc, metricFunc PanicMetricFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log the panic with stack trace
					if logFunc != nil {
						logFunc("Panic recovered: %v", err)
						logFunc("Stack trace:\n%s", debug.Stack())
					}

					// Record panic metric if provided
					if metricFunc != nil {
						metricFunc()
					}

					// Try to send error response
					// If headers were already written, this won't change the status code
					// but at least we logged the panic
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
