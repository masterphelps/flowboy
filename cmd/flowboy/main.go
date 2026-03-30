package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
	"github.com/masterphelps/flowboy/internal/tui"
	"github.com/masterphelps/flowboy/internal/web"
)

func main() {
	webMode := flag.Bool("web", false, "Start web UI")
	both := flag.Bool("both", false, "Start both TUI and web UI")
	headless := flag.Bool("headless", false, "Run headless (no UI)")
	port := flag.Int("port", 8042, "Web UI port")
	configPath := flag.String("config", "configs/flowboy.yaml", "Config file path")
	flag.Parse()

	// Determine mode
	mode := "tui" // default
	if *webMode {
		mode = "web"
	}
	if *both {
		mode = "both"
	}
	if *headless {
		mode = "headless"
	}

	fmt.Println("\u2554\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2557")
	fmt.Println("\u2551       F L O W B O Y  3000     \u2551")
	fmt.Println("\u255a\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u255d")
	fmt.Printf("Mode: %s | Port: %d | Config: %s\n\n", mode, *port, *configPath)

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Loaded %d machines, %d flows, %d collectors\n",
		len(cfg.Machines), len(cfg.Flows), len(cfg.Collectors))

	// Create and populate engine
	eng := engine.New()
	for _, mc := range cfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			log.Fatalf("Invalid machine %s: %v", mc.Name, err)
		}
		eng.AddMachine(m)
		fmt.Printf("  Machine: %s (%s/%d)\n", mc.Name, mc.IP, mc.Mask)
	}

	// Set global fluctuation if configured
	if cfg.Fluctuation != nil {
		eng.SetGlobalFluctuation(cfg.Fluctuation)
	}

	// Start engine
	eng.Start()

	// Add flows (after engine start so goroutines launch)
	for _, fc := range cfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			log.Fatalf("Invalid flow %s: %v", fc.Name, err)
		}
		if err := eng.AddFlow(f); err != nil {
			log.Fatalf("Failed to add flow %s: %v", f.Name, err)
		}
		fmt.Printf("  Flow: %s \u2192 %s (%s %s)\n", f.SourceName, f.DestName, f.Protocol, f.Rate)
	}

	// Create and start exporter
	exp, err := engine.NewExporter(eng.Records(), cfg.Collectors)
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}
	exp.Start()
	for _, c := range cfg.Collectors {
		fmt.Printf("  Collector: %s (%s, %s)\n", c.Name, c.Address, c.Version)
	}

	fmt.Printf("\nEngine running. %d flows active. Press Ctrl+C to stop.\n", eng.FlowCount())

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	switch mode {
	case "headless":
		<-sigCh
	case "tui":
		p := tea.NewProgram(tui.NewModel(eng, exp, cfg, *configPath), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	case "web":
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srv := web.NewServer(eng, exp, cfg, *configPath, *port)
		srv.StartBroadcast(ctx)
		fmt.Printf("Web UI: http://localhost:%d\n", *port)
		go func() {
			if err := srv.ListenAndServe(ctx); err != nil {
				log.Printf("Web server error: %v", err)
			}
		}()
		<-sigCh
	case "both":
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		srv := web.NewServer(eng, exp, cfg, *configPath, *port)
		srv.StartBroadcast(ctx)
		fmt.Printf("Web UI: http://localhost:%d\n", *port)
		go func() {
			if err := srv.ListenAndServe(ctx); err != nil {
				log.Printf("Web server error: %v", err)
			}
		}()
		p := tea.NewProgram(tui.NewModel(eng, exp, cfg, *configPath), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	}

	// Graceful shutdown
	fmt.Println("\nShutting down...")
	exp.Stop()
	eng.Stop()
	fmt.Println("Goodbye.")
}
