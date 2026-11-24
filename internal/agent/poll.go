package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/ivanhreyes/isphub-print-agent/internal/config"
)

// RunPoll pulls print jobs from ISPHub. It does nothing until serverURL is set.
// A 404 means that ISPHub does not ship the queue yet; the loop backs off
// instead of treating that as a fatal error.
func (s *Service) RunPoll(ctx context.Context, logf func(string, ...any)) {
	s.mu.Lock()
	base := s.Config.ServerURL
	s.mu.Unlock()
	if base == "" {
		return
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	wait := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := s.pollOnce(ctx)
		if err != nil {
			logf("cola de ISPHub: %v", err)
			if next < wait {
				next = 30 * time.Second
			}
		}
		if next <= 0 {
			next = wait
		}
		timer := time.NewTimer(next)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) pollOnce(ctx context.Context) (time.Duration, error) {
	s.mu.Lock()
	cfg := s.Config
	s.mu.Unlock()
	if cfg.AgentID == "" || cfg.AgentSecret == "" {
		if cfg.EnrollmentToken == "" {
			return time.Minute, fmt.Errorf("falta el token de alta del agente")
		}
		if err := s.enroll(ctx, cfg); err != nil {
			return 30 * time.Second, err
		}
		s.mu.Lock()
		cfg = s.Config
		s.mu.Unlock()
	}
	jobs, interval, err := s.pull(ctx, cfg)
	if err != nil {
		return interval, err
	}
	for _, job := range jobs {
		resultErr := s.printRemote(ctx, cfg, job)
		if postErr := s.postResult(ctx, cfg, job.ID, resultErr); postErr != nil {
			return interval, postErr
		}
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return interval, nil
}

type remoteJob struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Receipt  json.RawMessage `json:"receipt"`
	Document json.RawMessage `json:"document"`
}

func (s *Service) enroll(ctx context.Context, cfg config.File) error {
	host, _ := os.Hostname()
	payload, _ := json.Marshal(map[string]string{
		"token":    cfg.EnrollmentToken,
		"hostname": host,
		"version":  s.Version,
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
	})
	body, status, err := s.request(ctx, cfg, http.MethodPost, "/api/v1/print-agents/enroll", payload, false)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("ISPHub todavía no publica la cola de impresión")
	}
	if status >= 300 {
		return fmt.Errorf("el alta del agente respondió %d", status)
	}
	var parsed struct {
		Data struct {
			AgentID             string `json:"agentId"`
			AgentSecret         string `json:"agentSecret"`
			PollIntervalSeconds int    `json:"pollIntervalSeconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Data.AgentID == "" || parsed.Data.AgentSecret == "" {
		return fmt.Errorf("la respuesta de alta no trae credenciales")
	}
	s.mu.Lock()
	s.Config.AgentID = parsed.Data.AgentID
	s.Config.AgentSecret = parsed.Data.AgentSecret
	updated := s.Config
	s.mu.Unlock()
	if s.Save != nil {
		if err := s.Save(updated); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) pull(ctx context.Context, cfg config.File) ([]remoteJob, time.Duration, error) {
	body, status, err := s.request(ctx, cfg, http.MethodGet, "/api/v1/print-agents/jobs", nil, true)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusNotFound {
		return nil, 30 * time.Second, fmt.Errorf("ISPHub todavía no publica la cola de impresión")
	}
	if status >= 300 {
		return nil, 0, fmt.Errorf("la cola respondió %d", status)
	}
	var parsed struct {
		Data struct {
			PollIntervalSeconds int         `json:"pollIntervalSeconds"`
			Jobs                []remoteJob `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("la cola no se pudo leer")
	}
	return parsed.Data.Jobs, time.Duration(parsed.Data.PollIntervalSeconds) * time.Second, nil
}

func (s *Service) printRemote(ctx context.Context, cfg config.File, job remoteJob) error {
	payload := map[string]any{"kind": job.Kind}
	if len(job.Receipt) > 0 {
		payload["receipt"] = json.RawMessage(job.Receipt)
	}
	if len(job.Document) > 0 {
		payload["document"] = json.RawMessage(job.Document)
	}
	if job.Kind == "" {
		payload["kind"] = "payment-receipt"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	prepared, err := Prepare(raw, cfg.PaperWidthMm, s.Hostname)
	if err != nil {
		return err
	}
	if cfg.DryRun || s.Send == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.Send(sendCtx, cfg.PrinterHost, cfg.PrinterPort, prepared.Data)
}

func (s *Service) postResult(ctx context.Context, cfg config.File, id string, printErr error) error {
	if id == "" {
		return fmt.Errorf("el trabajo remoto no trae id")
	}
	body := map[string]string{"status": "printed"}
	if printErr != nil {
		message := printErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		body = map[string]string{"status": "failed", "error": message}
	}
	raw, _ := json.Marshal(body)
	_, status, err := s.request(ctx, cfg, http.MethodPost, "/api/v1/print-agents/jobs/"+id+"/result", raw, true)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("no se pudo reportar el trabajo %s (%d)", id, status)
	}
	return nil
}

func (s *Service) request(ctx context.Context, cfg config.File, method, path string, payload []byte, signed bool) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, cfg.ServerURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if signed {
		req.Header.Set("X-Agent-Id", cfg.AgentID)
		req.Header.Set("X-Agent-Secret", cfg.AgentSecret)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, nil
}
