package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/auth"
	"github.com/anomalyco/agents_manager/internal/config"
	"github.com/anomalyco/agents_manager/internal/logger"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/server"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/shell"
	"github.com/anomalyco/agents_manager/internal/skill"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

func main() {
	configPath := flag.String("config", "agents_manager.yaml", "path to configuration file")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	demo := flag.Bool("demo", false, "run pool demonstration and exit")
	repl := flag.Bool("repl", false, "start interactive REPL")
	serve := flag.Bool("serve", false, "start HTTP server (WebSocket + SPA)")
	flag.Parse()

	log := logger.New(*logLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Warn("config load warning, continuing with defaults", "error", err)
	} else {
		log.Info("configuration loaded", "path", *configPath)
	}

	log.Info("Agents Manager ready",
		"http_port", cfg.HTTP.Port,
		"ssh_port", cfg.SSH.Port,
		"home", cfg.Home,
		"workspaces", len(cfg.WorkspacePaths),
	)

	fmt.Fprintf(os.Stderr, "Agents Manager ready on :%d\n", cfg.HTTP.Port)

	if *demo {
		runDemo()
		return
	}

	if *repl {
		runREPL(cfg, log)
		return
	}

	if *serve {
		runServe(cfg, log)
		return
	}
}

type appServices struct {
	wsRegistry workspace.Registry
	agRegistry agent.Registry
	skRegistry *skill.Registry
	store      *session.Store
	pool       *pool.Pool
	router     *shell.Router
	auth       *auth.Manager
}

func initServices(cfg *config.Config, log *slog.Logger) *appServices {
	wsRegistry := workspace.NewRegistry()
	if err := wsRegistry.Discover(cfg.WorkspacePaths); err != nil {
		fmt.Fprintf(os.Stderr, "workspace discovery: %v\n", err)
	}

	agRegistry := agent.NewRegistry()
	if err := agRegistry.Discover(cfg.Home); err != nil {
		fmt.Fprintf(os.Stderr, "agent discovery: %v\n", err)
	}
	agRegistry.DiscoverSystem()

	skRegistry := skill.NewRegistry(cfg.Home)
	store := session.NewStore()

	poolLog := func(msg string, args ...any) {
		log.Debug(msg, args...)
	}
	agPool := pool.New(cfg.AgentPool.MaxInstances, poolLog)

	router := shell.NewRouter(store, wsRegistry, agRegistry, skRegistry, agPool)

	jwtSecret := make([]byte, 32)
	rand.Read(jwtSecret)

	accessTTL := time.Duration(cfg.Auth.AccessTokenExpiry) * time.Second
	refreshTTL := time.Duration(cfg.Auth.RefreshTokenExpiry) * time.Second
	lockoutDur := time.Duration(cfg.Auth.LockoutDuration) * time.Second

	authManager := auth.NewManager(
		jwtSecret, cfg.Auth.PasswordFile,
		accessTTL, refreshTTL,
		cfg.Auth.MaxLoginAttempts, lockoutDur,
	)

	return &appServices{
		wsRegistry: wsRegistry,
		agRegistry: agRegistry,
		skRegistry: skRegistry,
		store:      store,
		pool:       agPool,
		router:     router,
		auth:       authManager,
	}
}

func runServe(cfg *config.Config, log *slog.Logger) {
	svc := initServices(cfg, log)
	defer svc.pool.Shutdown()

	srv, err := server.Start(server.Config{
		Port:        cfg.HTTP.Port,
		Bind:        cfg.HTTP.Bind,
		StaticDir:   cfg.HTTP.StaticDir,
		AuthManager: svc.auth,
		Sessions:    svc.store,
		Workspaces:  svc.wsRegistry,
		Agents:      svc.agRegistry,
		Skills:      svc.skRegistry,
		Pool:        svc.pool,
		Router:      svc.router,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown error", "error", err)
	}
}

func runREPL(cfg *config.Config, log *slog.Logger) {
	fmt.Println("Initializing...")

	svc := initServices(cfg, log)
	defer svc.pool.Shutdown()

	sess := &session.Session{
		ID:        "repl-1",
		Transport: "repl",
		State:     session.StateIdle,
	}
	svc.store.Create(sess)

	fmt.Println("Agents Manager REPL")
	fmt.Println("Type 'help' for commands, Ctrl+C to exit.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			fmt.Print("> ")
			continue
		}

		ch := svc.router.Execute(sess.ID, input)
		for evt := range ch {
			prefix := ""
			switch evt.Type {
			case "error":
				prefix = "error: "
			case "ack":
				continue
			}
			fmt.Println(prefix + evt.Text)
		}

		time.Sleep(50 * time.Millisecond)
		fmt.Print("> ")
	}
}

func testdataPath(name string) string {
	paths := []string{
		filepath.Join("testdata", name),
	}
	exe, err := os.Executable()
	if err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "testdata", name))
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return name
}

func runDemo() {
	fmt.Println("\n=== Pool Demo ===")

	echoAgent := testdataPath("echo-agent.sh")
	timedAgent := testdataPath("timed-agent.sh")
	lingerAgent := testdataPath("linger-agent.sh")

	p := pool.New(2, nil)
	defer p.Shutdown()

	fmt.Println("\n1. Spawning echo-agent with 'hello from demo'...")
	ch, err := p.Submit("demo-1", echoAgent, nil, "", "hello from demo")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Submit error: %v\n", err)
		return
	}
	for evt := range ch {
		ts := evt.Timestamp.Format("15:04:05.000")
		fmt.Printf("   [%s] %-12s %s\n", ts, evt.Type, evt.Text)
	}
	fmt.Println("   -> agent completed")

	fmt.Println("\n2. Spawning linger-agent, killing after 500ms...")
	ch, err = p.Submit("demo-2", lingerAgent, nil, "", "kill demo")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Submit error: %v\n", err)
		return
	}
	time.Sleep(500 * time.Millisecond)
	if err := p.Kill("demo-2"); err != nil {
		fmt.Fprintf(os.Stderr, "Kill error: %v\n", err)
	}
	for evt := range ch {
		ts := evt.Timestamp.Format("15:04:05.000")
		fmt.Printf("   [%s] %-12s %s\n", ts, evt.Type, evt.Text)
	}
	fmt.Println("   -> agent killed")

	fmt.Println("\n3. Queue demo (pool max=2, submitting 3 timed agents with 1s each)...")
	ch1, _ := p.Submit("demo-3a", timedAgent, nil, "", "1")
	ch2, _ := p.Submit("demo-3b", timedAgent, nil, "", "1")
	ch3, err := p.Submit("demo-3c", timedAgent, nil, "", "1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Submit 3 error: %v\n", err)
	}
	fmt.Printf("   active=%d queued=%d\n", p.ActiveCount(), p.QueuedCount())

	drain := func(label string, ch <-chan pool.ResponseEvent) {
		for evt := range ch {
			ts := evt.Timestamp.Format("15:04:05.000")
			fmt.Printf("   [%s] %-12s %s\n", ts, evt.Type, evt.Text)
		}
		fmt.Printf("   -> %s done\n", label)
	}
	drain("A", ch1)
	drain("B", ch2)
	if ch3 != nil {
		drain("C", ch3)
	}

	fmt.Println("\n=== Demo complete ===")
}
