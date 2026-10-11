package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

// safeArg returns a value that is safe to format: an error's text, a scalar
// (including named kinds such as time.Duration), or the type name.
func safeArg(a any) any {
	if a == nil {
		return "<nil>"
	}
	rv := reflect.ValueOf(a)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "<nil>" // a typed-nil error would panic in Error()
	}
	if err, ok := a.(error); ok {
		return err.Error()
	}
	switch rv.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return a
	}
	return fmt.Sprintf("<%T>", a)
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
		safe[i] = safeArg(a)
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
