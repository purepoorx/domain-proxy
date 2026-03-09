package proxy

import (
	"log/slog"

	"domain-proxy/config"
)

type Rewriter struct {
	rules map[string]config.RewriteTarget // source -> target
}

func NewRewriter(rules map[string]config.RewriteTarget) *Rewriter {
	return &Rewriter{rules: rules}
}

func (r *Rewriter) Rewrite(hostname string) (target string, injectHeader bool, rewritten bool) {
	if t, ok := r.rules[hostname]; ok {
		slog.Info("rewrite domain", "from", hostname, "to", t.Host, "injectHeader", t.InjectHeader)
		return t.Host, t.InjectHeader, true
	}
	return hostname, false, false
}
