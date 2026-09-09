package game

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// AuthorizeAnnouncement reads current permissions, not a cached login claim.
func (ws *WorldService) AuthorizeAnnouncement(ctx context.Context, actor uuid.UUID) error {
	var raw []byte
	if err := ws.db.QueryRowContext(ctx, `SELECT permissions FROM users WHERE id=$1 AND is_active FOR SHARE`, actor).Scan(&raw); err != nil {
		return fmt.Errorf("active admin account required")
	}
	var permissions map[string]interface{}
	if err := json.Unmarshal(raw, &permissions); err != nil {
		return err
	}
	if permissions["admin"] != true {
		return fmt.Errorf("admin permission required")
	}
	return nil
}
