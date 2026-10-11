package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"rmbl/internal/database"
)

func TestWithContext_QueryErrorUsesContextLogger(t *testing.T) {
	var baseBuf, ctxBuf bytes.Buffer
	baseLog := zerolog.New(&baseBuf)
	db := database.DB.Session(&gorm.Session{Logger: database.NewGormLogger(&baseLog, database.SlowQueryThreshold)})
	svc := NewResourceService(db)

	ctxLog := zerolog.New(&ctxBuf).With().Str("request_id", "rid-svc").Logger()
	ctx := ctxLog.WithContext(context.Background())

	var x []map[string]any
	_ = svc.WithContext(ctx).db.Raw("SELECT * FROM no_such_table_63").Scan(&x).Error

	var m map[string]any
	assert.NoError(t, json.Unmarshal(bytes.TrimSpace(ctxBuf.Bytes()), &m), ctxBuf.String())
	assert.Equal(t, "rid-svc", m["request_id"])
	assert.Empty(t, baseBuf.String())
}
