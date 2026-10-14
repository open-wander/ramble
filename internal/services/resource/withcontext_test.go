package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"rmbl/internal/database"
	"rmbl/internal/services/logger"
)

func TestWithContext_QueryErrorUsesContextLogger(t *testing.T) {
	var baseBuf, ctxBuf bytes.Buffer
	baseLog := slog.New(slog.NewJSONHandler(&baseBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db := database.DB.Session(&gorm.Session{Logger: database.NewGormLogger(baseLog, database.SlowQueryThreshold)})
	svc := NewResourceService(db)

	ctxLog := slog.New(slog.NewJSONHandler(&ctxBuf, &slog.HandlerOptions{Level: slog.LevelDebug})).With(slog.String("request_id", "rid-svc"))
	ctx := logger.SWithContext(context.Background(), ctxLog)

	var x []map[string]any
	_ = svc.WithContext(ctx).db.Raw("SELECT * FROM no_such_table_63").Scan(&x).Error

	var m map[string]any
	assert.NoError(t, json.Unmarshal(bytes.TrimSpace(ctxBuf.Bytes()), &m), ctxBuf.String())
	assert.Equal(t, "rid-svc", m["request_id"])
	assert.Empty(t, baseBuf.String())
}
