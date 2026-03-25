package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	level string
	base  *log.Logger
	mu    sync.Mutex
}

func New(level string) *Logger {
	if level == "" {
		level = "info"
	}
	return &Logger{level: strings.ToLower(level), base: log.New(os.Stdout, "", 0)}
}

func (l *Logger) Info(msg string, kv ...any)  { l.write("info", msg, kv...) }
func (l *Logger) Warn(msg string, kv ...any)  { l.write("warn", msg, kv...) }
func (l *Logger) Error(msg string, kv ...any) { l.write("error", msg, kv...) }

func (l *Logger) write(level, msg string, kv ...any) {
	if !l.enabled(level) {
		return
	}
	rec := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "level": level, "msg": msg}
	for i := 0; i < len(kv)-1; i += 2 {
		k := fmt.Sprint(kv[i])
		v := kv[i+1]
		if redactedKey(k) {
			rec[k] = "<redacted>"
			continue
		}
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.base.Println(string(b))
}

func redactedKey(k string) bool {
	key := strings.ToLower(k)
	return strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "key")
}

func (l *Logger) enabled(level string) bool {
	order := map[string]int{"error": 0, "warn": 1, "info": 2, "debug": 3}
	return order[level] <= order[l.level]
}
