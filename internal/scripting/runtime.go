// Package scripting runs Starlark in disposable, credential-free subprocesses.
package scripting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const MaxSource = 16384
const MaxState = 8192
const MaxSteps = 50000

var slots = make(chan struct{}, 4)

type Input struct {
	Source   string            `json:"source"`
	Kind     string            `json:"kind"`
	Hook     string            `json:"hook"`
	Player   map[string]string `json:"player"`
	Room     map[string]string `json:"room"`
	Target   map[string]string `json:"target"`
	Command  string            `json:"command"`
	Text     string            `json:"text"`
	State    map[string]string `json:"state"`
	Validate bool              `json:"validate"`
}
type Message struct {
	Room bool   `json:"room"`
	Text string `json:"text"`
}
type Output struct {
	Messages []Message         `json:"messages"`
	State    map[string]string `json:"state"`
	Gold     int               `json:"gold"`
	Heal     int               `json:"heal"`
	Handled  bool              `json:"handled"`
	Error    string            `json:"error,omitempty"`
}

// init permits every gateway build (including test binaries) to be its own worker.
// The worker never initializes services or reads the gateway's environment.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "--rck-script-worker" {
		return
	}
	if err := limitWorker(); err != nil {
		os.Exit(2)
	}
	var in Input
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&in); err != nil {
		os.Exit(2)
	}
	out, err := evaluate(in)
	if err != nil {
		out = Output{Error: err.Error()}
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	os.Exit(0)
}

func Run(ctx context.Context, in Input) (Output, error) {
	if len(in.Source) > MaxSource {
		return Output{}, fmt.Errorf("script exceeds %d bytes", MaxSource)
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		return Output{}, fmt.Errorf("script workers busy; try again")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return Output{}, err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return Output{}, err
	}
	if len(data) > 65536 {
		return Output{}, fmt.Errorf("script input too large")
	}
	cmd := exec.CommandContext(ctx, executable, "--rck-script-worker")
	cmd.Env = []string{"GOMEMLIMIT=64MiB", "GOMAXPROCS=1", "GOTRACEBACK=none", "GORACE=atexit_sleep_ms=0"}
	cmd.Dir = os.TempDir()
	cmd.Stdin = bytes.NewReader(data)
	var stdout boundedBuffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Output{}, fmt.Errorf("script execution timed out or cancelled")
		}
		return Output{}, fmt.Errorf("script worker failed (resource limit or runtime failure)")
	}
	var out Output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Output{}, fmt.Errorf("invalid script result: %w", err)
	}
	if out.Error != "" {
		message := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, out.Error)
		if len(message) > 2048 {
			message = message[:2048] + "..."
		}
		return Output{}, fmt.Errorf("%s", message)
	}
	return out, nil
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, fmt.Errorf("script output limit exceeded")
	}
	return b.Buffer.Write(p)
}

func evaluate(in Input) (Output, error) {
	out := Output{State: make(map[string]string)}
	if len(in.Source) > MaxSource {
		return out, fmt.Errorf("source too large")
	}
	for k, v := range in.State {
		out.State[k] = v
	}
	thread := &starlark.Thread{Name: "game-script"}
	thread.SetMaxExecutionSteps(MaxSteps)
	// No Load callback: filesystem, networking and external modules are unavailable.
	thread.Print = func(t *starlark.Thread, msg string) { t.Cancel("use tell() or say() instead of print()") }
	initializing := true
	emit := func(room bool) *starlark.Builtin {
		name := "tell"
		if room {
			name = "say"
		}
		return starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if initializing {
				return nil, fmt.Errorf("put effects inside a hook function")
			}
			var msg string
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "text", &msg); err != nil {
				return nil, err
			}
			if len(out.Messages) >= 16 || len(msg) > 512 {
				return nil, fmt.Errorf("message limit exceeded (16 messages, 512 bytes each)")
			}
			if strings.IndexFunc(msg, unicode.IsControl) >= 0 {
				return nil, fmt.Errorf("messages cannot contain terminal control characters")
			}
			out.Messages = append(out.Messages, Message{Room: room, Text: msg})
			return starlark.None, nil
		})
	}
	pre := starlark.StringDict{"tell": emit(false), "say": emit(true)}
	for _, name := range []string{"award_gold", "heal"} {
		pre[name] = starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if initializing {
				return nil, fmt.Errorf("put effects inside a hook function")
			}
			var amount int
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "amount", &amount); err != nil {
				return nil, err
			}
			if amount < 0 || amount > 100 {
				return nil, fmt.Errorf("amount must be between 0 and 100")
			}
			value := &out.Gold
			if b.Name() == "heal" {
				value = &out.Heal
			}
			if *value+amount > 100 {
				return nil, fmt.Errorf("maximum 100 per effect per invocation")
			}
			*value += amount
			return starlark.None, nil
		})
	}
	pre["get_state"] = starlark.NewBuiltin("get_state", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var key, fallback string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &key, "default?", &fallback); err != nil {
			return nil, err
		}
		value, ok := out.State[key]
		if !ok {
			value = fallback
		}
		return starlark.String(value), nil
	})
	pre["set_state"] = starlark.NewBuiltin("set_state", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if initializing {
			return nil, fmt.Errorf("put effects inside a hook function")
		}
		var key, value string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &key, "value", &value); err != nil {
			return nil, err
		}
		if len(key) == 0 || len(key) > 64 || len(value) > 1024 {
			return nil, fmt.Errorf("state key/value too long")
		}
		out.State[key] = value
		data, _ := json.Marshal(out.State)
		if len(out.State) > 32 || len(data) > MaxState {
			return nil, fmt.Errorf("state limit exceeded")
		}
		return starlark.None, nil
	})
	globals, err := starlark.ExecFile(thread, "script.star", in.Source, pre)
	if err != nil {
		return Output{}, err
	}
	hooks := map[string]bool{"on_enter": true, "on_look": true, "on_say": true, "on_command": true, "on_use": true, "on_talk": true}
	count := 0
	for name, v := range globals {
		if hooks[name] {
			if fn, ok := v.(*starlark.Function); !ok || fn.NumParams() != 1 || fn.NumKwonlyParams() != 0 || fn.HasVarargs() || fn.HasKwargs() {
				return Output{}, fmt.Errorf("%s must be a function taking exactly one event argument", name)
			}
			if in.Kind == "" || ValidHook(in.Kind, name) {
				count++
			}
		}
	}
	if count == 0 {
		return Output{}, fmt.Errorf("define at least one on_* function supported by the script type")
	}
	if len(out.Messages) > 0 || len(out.State) != len(in.State) {
		return Output{}, fmt.Errorf("put effects inside a hook function")
	}
	// Discard all module-initialization effects, including same-sized state edits.
	out = Output{State: make(map[string]string)}
	for k, v := range in.State {
		out.State[k] = v
	}
	if in.Validate {
		return out, nil
	}
	initializing = false
	fn, ok := globals[in.Hook]
	if !ok {
		return out, nil
	}
	record := func(name string, m map[string]string) starlark.Value {
		d := starlark.StringDict{}
		for k, v := range m {
			d[k] = starlark.String(v)
		}
		return starlarkstruct.FromStringDict(starlark.String(name), d)
	}
	event := starlarkstruct.FromStringDict(starlark.String("event"), starlark.StringDict{
		"player": record("player", in.Player), "room": record("room", in.Room), "target": record("target", in.Target), "command": starlark.String(in.Command), "text": starlark.String(in.Text),
	})
	value, err := starlark.Call(thread, fn, starlark.Tuple{event}, nil)
	if err != nil {
		return Output{}, err
	}
	if value != starlark.None {
		handled, ok := value.(starlark.Bool)
		if !ok {
			return Output{}, fmt.Errorf("hooks must return True, False or None")
		}
		out.Handled = bool(handled)
	}
	return out, nil
}

// ValidHook defines the public event contract for each attachable object type.
func ValidHook(kind, hook string) bool {
	switch kind {
	case "room":
		return hook == "on_enter" || hook == "on_look" || hook == "on_say" || hook == "on_command"
	case "item":
		return hook == "on_use"
	case "npc":
		return hook == "on_talk"
	}
	return false
}
