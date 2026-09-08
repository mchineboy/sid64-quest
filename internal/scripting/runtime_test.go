package scripting

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkerStateMessagesAndLanguage(t *testing.T) {
	in := Input{Source: `def on_enter(event):
    visits = int(get_state("visits", "0")) + 1
    set_state("visits", str(visits))
    names = [x.upper() for x in [event.player.name, "friend"]]
    tell(", ".join(names))
    say("Visit " + str(visits))
    award_gold(5)
    heal(10)
    return True
`, Hook: "on_enter", Player: map[string]string{"name": "Ada"}, State: map[string]string{"visits": "3"}}
	out, err := Run(context.Background(), in)
	require.NoError(t, err)
	require.True(t, out.Handled)
	require.Equal(t, 5, out.Gold)
	require.Equal(t, 10, out.Heal)
	require.Equal(t, "4", out.State["visits"])
	require.Equal(t, []Message{{Text: "ADA, FRIEND"}, {Room: true, Text: "Visit 4"}}, out.Messages)
	require.Equal(t, "3", in.State["visits"])
}
func TestWorkerRejectsUnsafeOrInvalidPrograms(t *testing.T) {
	programs := map[string]string{
		"load": `load("/etc/passwd", "secret")
def on_enter(e): pass`,
		"filesystem": `def on_enter(e): open("/etc/passwd")`,
		"network":    `def on_enter(e): socket("example.com")`,
		"runaway": `def on_enter(e):
    for i in range(10000000):
        x = i + 1`,
		"output": `def on_enter(e):
    for i in range(17): tell("hello")`,
		"controls":    `def on_enter(e): tell("\x1b[2J")`,
		"large state": `def on_enter(e): set_state("x", "y" * 1025)`,
		"recursion":   `def on_enter(e): on_enter(e)`,
		"bad return":  `def on_enter(e): return "yes"`,
		"module effects": `tell("hello")
def on_enter(e): pass`,
		"reward limit": `def on_enter(e):
    award_gold(100)
    award_gold(1)`,
		"bad signature": `def on_enter(): pass`,
		"module state": `set_state("x", "y")
def on_enter(e): pass`,
		"no hooks": `x = 1`,
	}
	for name, source := range programs {
		t.Run(name, func(t *testing.T) {
			out, err := Run(context.Background(), Input{Source: source, Hook: "on_enter"})
			require.Error(t, err)
			require.Empty(t, out.Messages)
			require.Empty(t, out.State)
		})
	}
}
func TestWorkerValidationMissingHookAndCancellation(t *testing.T) {
	source := `def on_enter(e): tell("ok")`
	out, err := Run(context.Background(), Input{Source: source, Validate: true})
	require.NoError(t, err)
	require.Empty(t, out.Messages)
	out, err = Run(context.Background(), Input{Source: source, Hook: "on_look"})
	require.NoError(t, err)
	require.Empty(t, out.Messages)
	_, err = Run(context.Background(), Input{Source: strings.Repeat("#", MaxSource+1)})
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Run(ctx, Input{Source: source, Hook: "on_enter"})
	require.Error(t, err)
}

func TestWorkerMemoryLimitLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux kernel address-space limit")
	}
	_, err := Run(context.Background(), Input{Source: `def on_enter(e):
    x = list(range(300000000))`, Hook: "on_enter"})
	require.Error(t, err)
	out, err := Run(context.Background(), Input{Source: `def on_enter(e): tell("still alive")`, Hook: "on_enter"})
	require.NoError(t, err)
	require.Equal(t, "still alive", out.Messages[0].Text)
}

func TestErrorMessagesAreTerminalSafe(t *testing.T) {
	_, err := Run(context.Background(), Input{Source: `def on_enter(e): fail("\x1b[2Jbad")`, Hook: "on_enter"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "\x1b")
	_, err = Run(context.Background(), Input{Source: `def on_use(e): pass`, Kind: "room", Validate: true})
	require.Error(t, err)
}
