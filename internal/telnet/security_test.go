package telnet

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

func TestStoredCharacterNameCannotInjectControls(t *testing.T) {
	c, collect := drainedConnection(t)
	require.NoError(t, c.SendWhoList([]OnlinePlayer{{Name: "Eve\x1b[2J\n\u009b", Level: 1, Location: "Square"}}))
	output := collect()
	require.NotContains(t, output, "\x1b")
	require.NotContains(t, output, "\u009b")
	require.NotContains(t, output, "[2J\n")
}

func TestCoreIntegrationCredentialRevocation(t *testing.T) {
	for _, operation := range []string{"input", "poll", "replay"} {
		t.Run(operation, func(t *testing.T) {
			f := newCoreFixture(t)
			u, ch := f.player("Revoked Player")
			req := f.login(f.core, "revoked", u, ch)
			// A restored core must recheck the credential version before replay or input.
			replacement := NewCore(f.db, f.cache, f.cfg, logrus.New(), f.core.Token)
			_, err := f.db.Exec(`UPDATE users SET password_hash='changed' WHERE id=$1`, u)
			require.NoError(t, err)
			var response terminalwire.Response
			if operation == "replay" {
				response, err = replacement.dispatch(context.Background(), *req)
				require.NoError(t, err)
			} else {
				response = f.request(replacement, req, operation, "look")
			}
			require.True(t, response.Closed)
			require.Contains(t, outputText(response), "session has ended")
			require.NotContains(t, outputText(response), "Obvious exits")
			service := auth.NewAuthService(f.db, f.cache, f.cfg, logrus.New())
			c, _ := drainedConnection(t)
			c.State = StateInGame
			c.Session = &models.Session{UserID: u, AuthVersion: 1}
			server := &Server{authService: service}
			require.ErrorIs(t, server.checkSession(c), errQuit)
			valid, err := service.SessionActive(context.Background(), &models.Session{UserID: u, AuthVersion: 2})
			require.NoError(t, err)
			require.True(t, valid)
		})
	}
}
