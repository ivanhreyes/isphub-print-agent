package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/agent"
	"github.com/ivanhreyes/isphub-print-agent/internal/config"
	"github.com/ivanhreyes/isphub-print-agent/internal/discover"
	"github.com/ivanhreyes/isphub-print-agent/internal/launchd"
	"github.com/ivanhreyes/isphub-print-agent/internal/printer"
	"github.com/ivanhreyes/isphub-print-agent/internal/receipt"
)

var version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "discover":
		os.Exit(run(cmdDiscover))
	case "print-test":
		os.Exit(run(cmdPrintTest))
	case "print":
		os.Exit(run(cmdPrint))
	case "serve":
		os.Exit(run(cmdServe))
	case "install":
		os.Exit(run(cmdInstall))
	case "uninstall":
		os.Exit(run(cmdUninstall))
	case "status":
		os.Exit(run(cmdStatus))
	case "config":
		os.Exit(run(cmdConfig))
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func run(fn func([]string) error) int {
	if err := fn(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `Agente de impresión ISPHub para la EC Line EC-PM-80330 en Mac Apple Silicon.

Uso:
  isphub-print-agent discover
  isphub-print-agent print-test [--host IP] [--port 9100] [--paper 80]
  isphub-print-agent print --file recibo.json [--host IP]
  isphub-print-agent serve
  isphub-print-agent install
  isphub-print-agent uninstall
  isphub-print-agent status
  isphub-print-agent config --host IP [--port 9100] [--paper 80]
  isphub-print-agent version

El agente escucha solo en 127.0.0.1:8787. ISPHub, abierto en esta misma Mac,
puede enviarle el recibo de pago. La impresora tiene que estar en una IP
privada de la red local.
`)
}

func cmdDiscover(_ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	found, err := discover.Scan(ctx)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Println("No hay nada escuchando en el puerto 9100 de esta red.")
		return nil
	}
	for _, item := range found {
		fmt.Printf("%s\t%s\t%s\t%s\n", item.Host, item.Role, item.Label, item.Model)
	}
	return nil
}

func cmdPrintTest(args []string) error {
	fs := flag.NewFlagSet("print-test", flag.ContinueOnError)
	host := fs.String("host", "", "IP de la impresora")
	port := fs.Int("port", 0, "puerto TCP, normalmente 9100")
	paper := fs.Int("paper", 0, "58 o 80")
	dry := fs.Bool("dry-run", false, "no enviar a la impresora")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	targetHost, targetPort, targetPaper := pickTarget(cfg, *host, *port, *paper)
	name, _ := os.Hostname()
	data, err := receipt.TestPage(name, targetPaper)
	if err != nil {
		return err
	}
	if *dry || cfg.DryRun {
		fmt.Printf("dry-run: %d bytes para %s:%d\n", len(data), targetHost, targetPort)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := printer.Send(ctx, targetHost, targetPort, data); err != nil {
		return err
	}
	fmt.Printf("Página de prueba enviada a %s:%d\n", targetHost, targetPort)
	return nil
}

func cmdPrint(args []string) error {
	fs := flag.NewFlagSet("print", flag.ContinueOnError)
	file := fs.String("file", "", "JSON del recibo de ISPHub")
	host := fs.String("host", "", "IP de la impresora")
	port := fs.Int("port", 0, "puerto TCP")
	dry := fs.Bool("dry-run", false, "mostrar el texto y no imprimir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("indica el recibo con --file")
	}
	body, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name, _ := os.Hostname()
	prepared, err := agent.Prepare(body, cfg.PaperWidthMm, name)
	if err != nil {
		return err
	}
	targetHost, targetPort, _ := pickTarget(cfg, *host, *port, prepared.Paper)
	if *dry || cfg.DryRun {
		fmt.Println(prepared.Preview)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := printer.Send(ctx, targetHost, targetPort, prepared.Data); err != nil {
		return err
	}
	fmt.Printf("Recibo enviado a %s:%d\n", targetHost, targetPort)
	return nil
}

func cmdServe(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name, _ := os.Hostname()
	svc := &agent.Service{
		Config:   cfg,
		Version:  version,
		Hostname: name,
		Save:     config.Save,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go svc.RunPoll(ctx, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	})
	fmt.Printf("ISPHub print agent %s escuchando en http://%s\n", version, cfg.Listen)
	return svc.Listen(ctx)
}

func cmdInstall(_ []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := config.Load(); err != nil {
		return err
	}
	path, pathErr := config.Path()
	if pathErr != nil {
		return pathErr
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err := config.Save(config.Default()); err != nil {
			return err
		}
	}
	if err := launchd.Install(exe); err != nil {
		return err
	}
	fmt.Println("Agente instalado. Arranca con la sesión y escucha en 127.0.0.1:8787.")
	fmt.Println("Si aún no elegiste impresora: isphub-print-agent discover")
	return nil
}

func cmdUninstall(_ []string) error {
	if err := launchd.Uninstall(); err != nil {
		return err
	}
	fmt.Println("Agente detenido. La configuración se conservó.")
	return nil
}

func cmdStatus(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(cfg.Redacted(), "", "  ")
	fmt.Println(string(raw))
	out, err := exec.Command("launchctl", "print", launchd.ServiceTarget()).CombinedOutput()
	if err != nil {
		fmt.Println("LaunchAgent: no está cargado")
		return nil
	}
	fmt.Printf("LaunchAgent: %s\n", launchd.ServiceTarget())
	if len(out) > 0 {
		fmt.Println("launchctl print respondió")
	}
	return nil
}

func cmdConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	host := fs.String("host", "", "IP privada de la EC-PM-80330")
	port := fs.Int("port", 0, "puerto TCP")
	paper := fs.Int("paper", 0, "58 o 80")
	server := fs.String("server", "", "URL https de ISPHub")
	token := fs.String("token", "", "token de alta, de un solo uso")
	listen := fs.String("listen", "", "dirección local, por ejemplo 127.0.0.1:8787")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil && !os.IsNotExist(err) {
		// Load returns Default when the file is missing, so a real error is fatal.
		return err
	}
	if *host != "" {
		cfg.PrinterHost = *host
	}
	if *port != 0 {
		cfg.PrinterPort = *port
	}
	if *paper != 0 {
		cfg.PaperWidthMm = *paper
	}
	if *server != "" {
		cfg.ServerURL = *server
	}
	if *token != "" {
		cfg.EnrollmentToken = *token
		cfg.AgentID = ""
		cfg.AgentSecret = ""
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Println("Configuración guardada.")
	return nil
}

func pickTarget(cfg config.File, host string, port, paper int) (string, int, int) {
	if host == "" {
		host = cfg.PrinterHost
	}
	if port == 0 {
		port = cfg.PrinterPort
	}
	if port == 0 {
		port = 9100
	}
	if paper == 0 {
		paper = cfg.PaperWidthMm
	}
	if paper != 58 {
		paper = 80
	}
	return host, port, paper
}
