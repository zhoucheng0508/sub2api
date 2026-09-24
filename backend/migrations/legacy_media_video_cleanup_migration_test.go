package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyMediaVideoPreparationMigrationIsRollingSafe(t *testing.T) {
	content, err := FS.ReadFile("238_prepare_legacy_media_video_cleanup.sql")
	require.NoError(t, err)

	sql := strings.ToUpper(strings.Join(strings.Fields(string(content)), " "))
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS LEGACY_MEDIA_VIDEO_CLEANUP_REFUNDS")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "UPDATE USERS")
}
