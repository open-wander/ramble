package logger

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// RequestLogger logs one http.request line per request through slog and
// attaches a request-scoped logger carrying request_id to the request's user
// context, read back with logger.FromContext(c.UserContext()). It must run
// after the requestid middleware.
//
// path is c.Path(), which excludes the query string, so tokens passed in a
// URL never reach the logs. 5xx responses log at ERROR, everything else at
// INFO, so scanner 404s stay out of the error stream.
func RequestLogger(base *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		rid, _ := c.Locals(requestid.ConfigDefault.ContextKey).(string)
		reqLog := base.With(slog.String("request_id", rid))
		c.SetUserContext(WithContext(c.UserContext(), reqLog))

		// Run the error handler here, as fiber's own logger middleware does,
		// so the logged status is the one the client receives.
		if err := c.Next(); err != nil {
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()
		reqLog.Log(c.UserContext(), levelForStatus(status), "http.request",
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("client_ip", c.IP()))
		return nil
	}
}

func levelForStatus(status int) slog.Level {
	if status >= fiber.StatusInternalServerError {
		return slog.LevelError
	}
	return slog.LevelInfo
}
