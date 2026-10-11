package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"rmbl/internal/services/logger"
)

// SlowQueryThreshold is the duration above which a query is logged as slow.
const SlowQueryThreshold = 200 * time.Millisecond

// gormLogger writes GORM events through zerolog. Successful fast queries are
// never logged; slow queries go out at WARN as db.slow_query and failures
// (other than gorm.ErrRecordNotFound) at ERROR as db.query_error. SQL is
// logged with placeholders, never with bound values, so user data stays out
// of the logs. Info/Warn/Error args are rendered into text and never
// serialised.
type gormLogger struct {
	base          *zerolog.Logger
	slowThreshold time.Duration
	level         gormlogger.LogLevel
}

// NewGormLogger returns a GORM logger that writes to base, or to the logger
// attached to the query's context when there is one (so request-scoped
// queries carry request_id).
func NewGormLogger(base *zerolog.Logger, slowThreshold time.Duration) gormlogger.Interface {
	return &gormLogger{base: base, slowThreshold: slowThreshold, level: gormlogger.Warn}
}

func (g *gormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *g
	c.level = level
	return &c
}

func (g *gormLogger) logger(ctx context.Context) *zerolog.Logger {
	return logger.FromContextOr(ctx, g.base)
}

// formatText renders a GORM message with its args. Only errors, strings and
// scalars are formatted; anything else (structs such as pgconn.Config, which
// holds the password) is replaced by its type name.
func formatText(msg string, args []any) string {
	if len(args) == 0 {
		return msg
	}
	safe := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case error:
			safe[i] = v.Error()
		case string, bool,
			int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float32, float64:
			safe[i] = v
		default:
			safe[i] = fmt.Sprintf("<%T>", a)
		}
	}
	return fmt.Sprintf(msg, safe...)
}

func (g *gormLogger) Info(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Info {
		g.logger(ctx).Info().Str("text", formatText(msg, args)).Msg("db.info")
	}
}

func (g *gormLogger) Warn(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Warn {
		g.logger(ctx).Warn().Str("text", formatText(msg, args)).Msg("db.warn")
	}
}

func (g *gormLogger) Error(ctx context.Context, msg string, args ...any) {
	if g.level >= gormlogger.Error {
		g.logger(ctx).Error().Str("text", formatText(msg, args)).Msg("db.error")
	}
}

func (g *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	elapsedMS := float64(elapsed.Microseconds()) / 1000

	switch {
	case err != nil && g.level >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		g.logger(ctx).Error().Err(err).
			Float64("elapsed_ms", elapsedMS).
			Int64("rows", rows).
			Str("sql", sql).
			Msg("db.query_error")
	case g.slowThreshold > 0 && elapsed > g.slowThreshold && g.level >= gormlogger.Warn:
		sql, rows := fc()
		g.logger(ctx).Warn().
			Float64("elapsed_ms", elapsedMS).
			Int64("rows", rows).
			Str("sql", sql).
			Msg("db.slow_query")
	}
}

// ParamsFilter stops GORM interpolating bound values into the SQL handed to
// Trace, so logged statements keep their $n placeholders.
func (g *gormLogger) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}

// newDefaultGormLogger is the production configuration, writing through the
// global zerolog logger.
func newDefaultGormLogger() gormlogger.Interface {
	return NewGormLogger(&logger.Log, SlowQueryThreshold)
}
