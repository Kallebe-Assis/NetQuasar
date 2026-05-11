package api

import (
	"context"
	"strings"
)

func (s *Server) setMonitoringActivity(ctx context.Context, activity string) {
	activity = strings.TrimSpace(activity)
	if s.DB() == nil {
		return
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE monitoring_runtime
		SET last_activity = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN current_activity
				ELSE last_activity
			END,
			last_activity_finished_at = CASE
				WHEN NULLIF($1, '') IS NULL AND current_activity IS NOT NULL THEN now()
				ELSE last_activity_finished_at
			END,
			activity_started_at = CASE
				WHEN NULLIF($1, '') IS NULL THEN NULL
				WHEN current_activity IS DISTINCT FROM NULLIF($1, '') THEN now()
				ELSE activity_started_at
			END,
			current_activity = NULLIF($1, ''),
			activity_updated_at = CASE WHEN NULLIF($1, '') IS NULL THEN NULL ELSE now() END,
			updated_at = now()
		WHERE id = 1
	`, activity)
}
