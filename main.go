package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"domain-proxy/cert"
	"domain-proxy/config"
	"domain-proxy/proxy"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "gencert":
		runGenCert(os.Args[2:])
	case "run":
		runProxy(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  domain-proxy gencert [--out DIR]     Generate CA certificate
  domain-proxy run     [--config FILE] Start proxy server
`)
}

func runGenCert(args []string) {
	fs := flag.NewFlagSet("gencert", flag.ExitOnError)
	outDir := fs.String("out", "./certs", "output directory for CA cert and key")
	fs.Parse(args)

	if err := cert.GenerateCA(*outDir); err != nil {
		slog.Error("generate CA failed", "error", err)
		os.Exit(1)
	}
}

func runProxy(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "./config/config.yaml", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	certStore, err := cert.NewStore(cfg.TLS.CACert, cfg.TLS.CAKey)
	if err != nil {
		slog.Error("load CA certificate failed", "error", err)
		os.Exit(1)
	}

	ruleMap := cfg.BuildRuleMap()
	rewriter := proxy.NewRewriter(ruleMap)

	slog.Info("loaded rewrite rules", "count", len(ruleMap), "defaultTarget", cfg.DefaultTarget)
	for src, rt := range ruleMap {
		slog.Info("rule", "source", src, "target", rt.Host, "injectHeader", rt.InjectHeader)
	}

	srv := proxy.NewServer(cfg.Proxy.Addr, certStore, rewriter)
	if err := srv.Start(); err != nil {
		slog.Error("proxy stopped", "error", err)
		os.Exit(1)
	}
}
