package proxy

import (
	"log/slog"
)

type Rewriter struct {
	rules map[string]string // source -> target
}

func NewRewriter(rules map[string]string) *Rewriter {
	return &Rewriter{rules: rules}
}

func (r *Rewriter) Rewrite(hostname string) (target string, rewritten bool) {
	if t, ok := r.rules[hostname]; ok {
		slog.Info("rewrite domain", "from", hostname, "to", t)
		return t, true
	}
	return hostname, false
}
