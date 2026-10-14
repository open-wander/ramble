package logger

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog"
)

// RequestLogger logs one http.request line per request through zerolog and
// attaches request-scoped loggers carrying request_id to the request's user
// context: base's as logger.FromContext(c.UserContext()), sbase's as
// logger.SFromContext(c.UserContext()). It must run after the requestid
// middleware.
//
// path is c.Path(), which excludes the query string, so tokens passed in a
// URL never reach the logs. 5xx responses log at ERROR, everything else at
// INFO, so scanner 404s stay out of the error stream.
func RequestLogger(base *zerolog.Logger, sbase *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		rid, _ := c.Locals(requestid.ConfigDefault.ContextKey).(string)
		reqLog := base.With().Str("request_id", rid).Logger()
		c.SetUserContext(SWithContext(reqLog.WithContext(c.UserContext()), sbase.With(slog.String("request_id", rid))))

		// Run the error handler here, as fiber's own logger middleware does,
		// so the logged status is the one the client receives.
		if err := c.Next(); err != nil {
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()
		reqLog.WithLevel(levelForStatus(status)).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Float64("duration_ms", float64(time.Since(start).Microseconds())/1000).
			Str("client_ip", c.IP()).
			Msg("http.request")
		return nil
	}
}

func levelForStatus(status int) zerolog.Level {
	if status >= fiber.StatusInternalServerError {
		return zerolog.ErrorLevel
	}
	return zerolog.InfoLevel
}
